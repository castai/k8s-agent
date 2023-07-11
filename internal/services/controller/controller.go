//go:generate mockgen -destination ./mock/workqueue.go k8s.io/client-go/util/workqueue Interface
package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	rbacclientv1 "k8s.io/client-go/kubernetes/typed/rbac/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"k8s.io/metrics/pkg/client/clientset/versioned"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/controller/delta"
	"castai-agent/internal/services/controller/handlers/filters"
	"castai-agent/internal/services/controller/handlers/filters/autoscalerevents"
	"castai-agent/internal/services/controller/handlers/filters/oomevents"
	custominformers "castai-agent/internal/services/controller/informers"
	"castai-agent/internal/services/providers/types"
	"castai-agent/internal/services/version"
	"castai-agent/pkg/labels"
)

type Controller struct {
	log          logrus.FieldLogger
	clusterID    string
	castaiclient castai.Client
	provider     types.Provider
	queue        workqueue.Interface
	cfg          *config.Controller
	informers    map[reflect.Type]*custominformers.HandledInformer

	discovery       discovery.DiscoveryInterface
	metricsClient   versioned.Interface
	informerFactory informers.SharedInformerFactory

	delta   *delta.Delta
	deltaMu sync.Mutex

	triggerRestart func()

	agentVersion    *config.AgentVersion
	healthzProvider *HealthzProvider

	conditionalInformers []conditionalInformer
	rbacV1               rbacclientv1.RbacV1Interface
}

type conditionalInformer struct {
	// name is the name of the API resource, ex.: "poddisruptionbudgets"
	name string
	// groupVersion is the group and version of the API type, ex.: "policy/authorizationv1"
	groupVersion string
	// apiType is the type of the API object, ex.: "*authorizationv1.PodDisruptionBudget"
	apiType reflect.Type
	// informer is the informer for the API type
	informer cache.SharedInformer
	// permissionVerbs is the list of verbs required to watch the API type
	permissionVerbs []string
}

func New(
	log logrus.FieldLogger,
	f informers.SharedInformerFactory,
	discovery discovery.DiscoveryInterface,
	castaiclient castai.Client,
	metricsClient versioned.Interface,
	provider types.Provider,
	clusterID string,
	cfg *config.Controller,
	v version.Interface,
	agentVersion *config.AgentVersion,
	healthzProvider *HealthzProvider,
) *Controller {
	healthzProvider.Initializing()

	queue := workqueue.NewNamed("castai-agent")

	defaultInformers := map[reflect.Type]cache.SharedInformer{
		reflect.TypeOf(&corev1.Node{}):                  f.Core().V1().Nodes().Informer(),
		reflect.TypeOf(&corev1.Pod{}):                   f.Core().V1().Pods().Informer(),
		reflect.TypeOf(&corev1.PersistentVolume{}):      f.Core().V1().PersistentVolumes().Informer(),
		reflect.TypeOf(&corev1.PersistentVolumeClaim{}): f.Core().V1().PersistentVolumeClaims().Informer(),
		reflect.TypeOf(&corev1.ReplicationController{}): f.Core().V1().ReplicationControllers().Informer(),
		reflect.TypeOf(&corev1.Namespace{}):             f.Core().V1().Namespaces().Informer(),
		reflect.TypeOf(&corev1.Service{}):               f.Core().V1().Services().Informer(),
		reflect.TypeOf(&appsv1.Deployment{}):            f.Apps().V1().Deployments().Informer(),
		reflect.TypeOf(&appsv1.ReplicaSet{}):            f.Apps().V1().ReplicaSets().Informer(),
		reflect.TypeOf(&appsv1.DaemonSet{}):             f.Apps().V1().DaemonSets().Informer(),
		reflect.TypeOf(&appsv1.StatefulSet{}):           f.Apps().V1().StatefulSets().Informer(),
		reflect.TypeOf(&storagev1.StorageClass{}):       f.Storage().V1().StorageClasses().Informer(),
		reflect.TypeOf(&batchv1.Job{}):                  f.Batch().V1().Jobs().Informer(),
	}

	conditionalInformers := []conditionalInformer{
		{
			name:            "poddisruptionbudgets",
			groupVersion:    policyv1.SchemeGroupVersion.String(),
			apiType:         reflect.TypeOf(&policyv1.PodDisruptionBudget{}),
			informer:        f.Policy().V1().PodDisruptionBudgets().Informer(),
			permissionVerbs: []string{"list", "watch"},
		},
		{
			name:            "csinodes",
			groupVersion:    storagev1.SchemeGroupVersion.String(),
			apiType:         reflect.TypeOf(&storagev1.CSINode{}),
			informer:        f.Storage().V1().CSINodes().Informer(),
			permissionVerbs: []string{"get", "list", "watch"},
		},
		{
			name:            "horizontalpodautoscalers",
			groupVersion:    autoscalingv1.SchemeGroupVersion.String(),
			apiType:         reflect.TypeOf(&autoscalingv1.HorizontalPodAutoscaler{}),
			informer:        f.Autoscaling().V1().HorizontalPodAutoscalers().Informer(),
			permissionVerbs: []string{"get", "list", "watch"},
		},
	}

	handledInformers := map[reflect.Type]*custominformers.HandledInformer{}
	for typ, informer := range defaultInformers {
		handledInformers[typ] = custominformers.NewHandledInformer(log, queue, informer, typ, nil)
	}

	eventType := reflect.TypeOf(&corev1.Event{})
	handledInformers[eventType] = custominformers.NewHandledInformer(
		log,
		queue,
		f.Core().V1().Events().Informer(),
		eventType,
		filters.Filters{
			{
				autoscalerevents.Filter,
			},
			{
				oomevents.Filter,
			},
		},
	)

	return &Controller{
		log:                  log,
		clusterID:            clusterID,
		castaiclient:         castaiclient,
		provider:             provider,
		cfg:                  cfg,
		delta:                delta.New(log, clusterID, v.Full()),
		queue:                queue,
		informers:            handledInformers,
		agentVersion:         agentVersion,
		healthzProvider:      healthzProvider,
		metricsClient:        metricsClient,
		discovery:            discovery,
		informerFactory:      f,
		conditionalInformers: conditionalInformers,
	}
}

func (c *Controller) Run(ctx context.Context) error {
	defer c.queue.ShutDown()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	c.triggerRestart = cancel

	syncs := make([]cache.InformerSynced, 0, len(c.informers))
	for objType, informer := range c.informers {
		objType := objType
		informer := informer
		syncs = append(syncs, func() bool {
			hasSynced := informer.HasSynced()
			if !hasSynced {
				c.log.Infof("Informer cache for %v has not been synced.", objType.String())
			}

			return hasSynced
		})
	}

	waitStartedAt := time.Now()
	c.log.Info("waiting for informers cache to sync")
	if !cache.WaitForCacheSync(ctx.Done(), syncs...) {
		c.log.Error("failed to sync")
		return fmt.Errorf("failed to wait for cache sync")
	}
	c.log.Infof("informers cache synced after %v", time.Since(waitStartedAt))

	go func() {
		const dur = 15 * time.Second
		c.log.Infof("polling agent configuration every %s", dur)
		wait.Until(func() {
			req := &castai.AgentTelemetryRequest{
				AgentVersion: c.agentVersion.Version,
				GitCommit:    c.agentVersion.GitCommit,
			}
			cfg, err := c.castaiclient.ExchangeAgentTelemetry(ctx, c.clusterID, req)
			if err != nil {
				c.log.Errorf("failed getting agent configuration: %v", err)
				return
			}
			// Resync only when at least one full snapshot has already been sent.
			if cfg.Resync && !c.delta.FullSnapshot {
				c.log.Info("restarting controller to resync data")
				c.triggerRestart()
			}
		}, dur, ctx.Done())
	}()

	go func() {
		if err := c.collectInitialSnapshot(ctx); err != nil {
			const maxItems = 5
			queueContent := c.debugQueueContent(maxItems)
			log := c.log.WithField("queue_content", queueContent)
			// Crash agent in case it's not able to collect full snapshot from informers cache.
			// TODO (CO-1632): refactor crashing to "normal" exit or healthz metric; abruptly
			//  stopping the agent does not give it a chance to release leader lock.
			log.Fatalf("error while collecting initial snapshot: %v", err)
		}

		// Since both initial snapshot collection and event handlers writes to the same delta queue add
		// some sleep to prevent sending few large deltas on initial agent startup.
		c.log.Infof("sleeping for %s before starting to send cluster deltas", c.cfg.InitialSleepDuration)
		time.Sleep(c.cfg.InitialSleepDuration)

		c.healthzProvider.Initialized()

		c.log.Infof("sending cluster deltas every %s", c.cfg.Interval)
		wait.Until(func() {
			c.send(ctx)
		}, c.cfg.Interval, ctx.Done())
	}()

	go c.startConditionalInformersWithWatcher(ctx, c.conditionalInformers)

	podMetricsType := reflect.TypeOf(&v1beta1.PodMetrics{})
	go c.startPodMetricsInformersWithWatcher(ctx, podMetricsType)

	go func() {
		<-ctx.Done()
		c.queue.ShutDown()
	}()

	c.pollQueueUntilShutdown()

	return nil
}

func (c *Controller) startConditionalInformersWithWatcher(ctx context.Context, conditionalInformers []conditionalInformer) {
	if err := wait.PollImmediateInfiniteWithContext(ctx, time.Minute*2, func(ctx context.Context) (done bool, err error) {
		apiResourceLists := fetchApiResourceLists(c.discovery, c.log)
		if apiResourceLists == nil {
			return false, nil
		}
		c.log.Infof("Cluster API server is available, trying to start conditional informers")

		for _, informer := range conditionalInformers {
			apiResourceListForGroupVersion := getApiResourceListByGroupVersion(informer.groupVersion, apiResourceLists)

			resourceAvailable := isResourceAvailable(informer.apiType, apiResourceListForGroupVersion)
			informerHaveAccess := c.informerHaveAccess(apiResourceListForGroupVersion, informer)

			if resourceAvailable && informerHaveAccess {
				c.log.Infof("Starting conditional informer for %v", informer.name)
				custominformers.NewHandledInformer(c.log, c.queue, informer.informer, informer.apiType, nil)
			} else {
				c.log.Infof("Skipping conditional informer name: %v, API resource available: %t, has required access: %t",
					informer.name,
					resourceAvailable,
					informerHaveAccess,
				)
			}
		}
		return true, nil
	}); err != nil {
		c.log.Warnf("Error when waiting for server resources: %v", err.Error())
	}
}

func (c *Controller) startPodMetricsInformersWithWatcher(ctx context.Context, podMetricsType reflect.Type) {
	if err := wait.PollImmediateInfiniteWithContext(ctx, time.Minute*2, func(ctx context.Context) (done bool, err error) {
		apiResourceLists := fetchApiResourceLists(c.discovery, c.log)
		if apiResourceLists == nil {
			return false, nil
		}

		apiResourceListForGroupVersion := getApiResourceListByGroupVersion(v1beta1.SchemeGroupVersion.String(), apiResourceLists)
		if !isResourceAvailable(podMetricsType, apiResourceListForGroupVersion) {
			return false, nil
		}

		c.log.Infof("Cluster supports pod metrics, will start collecting them.")
		metricsInformer := c.informerFactory.InformerFor(&v1beta1.PodMetrics{}, func(_ kubernetes.Interface, _ time.Duration) cache.SharedIndexInformer {
			return custominformers.NewPodMetricsInformer(c.log, c.metricsClient)
		})

		informer := custominformers.NewHandledInformer(c.log, c.queue, metricsInformer, podMetricsType, nil)
		informer.Run(ctx.Done())

		return true, nil
	}); err != nil {
		c.log.Warnf("Error when polling resource availability: %v", err.Error())
	}
}

// collectInitialSnapshot is used to add a time buffer to collect the initial snapshot which is larger than periodic
// delta because it contains a significant portion of the Kubernetes state.
func (c *Controller) collectInitialSnapshot(ctx context.Context) error {
	c.log.Info("collecting initial cluster snapshot")

	startedAt := time.Now()

	ctx, cancel := context.WithTimeout(ctx, c.cfg.PrepTimeout)
	defer cancel()

	// Collect initial state from cached informers and push to deltas queue.
	for _, informer := range c.informers {
		for _, item := range informer.GetStore().List() {
			informer.Handler.OnAdd(item)
		}
	}

	cond := func() (done bool, err error) {
		queueLen := c.queue.Len()
		log := c.log.WithField("queue_length", queueLen)
		log.Debug("waiting until initial queue empty")

		if queueLen == 0 {
			c.log.Infof("done waiting for initial cluster snapshot collection after %v", time.Since(startedAt))
			return true, nil
		}

		return false, nil
	}

	if err := wait.PollImmediateUntil(time.Second, cond, ctx.Done()); err != nil {
		return fmt.Errorf("waiting for initial snapshot collection: %w", err)
	}
	return nil
}

func (c *Controller) pollQueueUntilShutdown() {
	for {
		i, shutdown := c.queue.Get()
		if shutdown {
			return
		}
		c.processItem(i)
	}
}

func (c *Controller) processItem(i interface{}) {
	defer c.queue.Done(i)

	di, ok := i.(*delta.Item)
	if !ok {
		c.log.Errorf("expected queue item to be of type %T but got %T", &delta.Item{}, i)
		return
	}

	c.deltaMu.Lock()
	c.delta.Add(di)
	c.deltaMu.Unlock()
}

func (c *Controller) send(ctx context.Context) {
	c.deltaMu.Lock()
	defer c.deltaMu.Unlock()

	nodesByName := map[string]*corev1.Node{}
	var nodes []*corev1.Node

	for _, item := range c.delta.Cache {
		n, ok := item.Obj.(*corev1.Node)
		if !ok {
			continue
		}

		nodesByName[n.Name] = n
		nodes = append(nodes, n)
	}

	if len(nodes) > 0 {
		spots, err := c.provider.FilterSpot(ctx, nodes)
		if err != nil {
			c.log.Warnf("failed to determine node lifecycle, some functionality might be limited: %v", err)
		}

		for _, spot := range spots {
			nodesByName[spot.Name].Labels[labels.CastaiFakeSpot] = "true"
		}
	}

	if err := c.castaiclient.SendDelta(ctx, c.clusterID, c.delta.ToCASTAIRequest()); err != nil {
		if !errors.Is(err, context.Canceled) {
			c.log.Errorf("failed sending delta: %v", err)
		}

		if errors.Is(err, castai.ErrInvalidContinuityToken) {
			c.log.Info("restarting controller due to continuity token mismatch")
			c.triggerRestart()
		}

		return
	}

	c.healthzProvider.SnapshotSent()

	c.delta.Clear()
}

func (c *Controller) debugQueueContent(maxItems int) string {
	l := c.queue.Len()
	if l > maxItems {
		l = maxItems
	}
	queueItems := make([]interface{}, l)
	for i := 0; i < l; i++ {
		item, done := c.queue.Get()
		if done {
			break
		}
		queueItems[i] = item
	}
	bytes, err := json.Marshal(queueItems)
	content := string(bytes)
	if err != nil {
		content = "err: " + err.Error()
	}

	return content
}

func (c *Controller) informerHaveAccess(apiResourceList *metav1.APIResourceList, informer conditionalInformer) bool {
	_, ok := lo.Find(apiResourceList.APIResources, func(apiResource metav1.APIResource) bool {
		intersect := lo.Intersect(apiResource.Verbs, informer.permissionVerbs)
		return len(intersect) == len(informer.permissionVerbs) || slices.Contains(apiResource.Verbs, "*")
	})
	return ok
}

func (c *Controller) getMissingConditionalInformers() []conditionalInformer {
	var missingInformers []conditionalInformer
	for _, informer := range c.conditionalInformers {
		if _, ok := c.informers[informer.apiType]; !ok {
			missingInformers = append(missingInformers, informer)
		}
	}
	return missingInformers
}

func fetchApiResourceLists(client discovery.DiscoveryInterface, log logrus.FieldLogger) []*metav1.APIResourceList {
	_, apiResourceLists, err := client.ServerGroupsAndResources()
	if err != nil {
		log.Warnf("Error when getting server resources: %v", err.Error())
		return nil
	}
	return apiResourceLists
}

func getApiResourceListByGroupVersion(groupVersion string, apiResourceLists []*metav1.APIResourceList) *metav1.APIResourceList {
	for _, apiResourceList := range apiResourceLists {
		if apiResourceList.GroupVersion == groupVersion {
			return apiResourceList
		}
	}
	// return empty list if not found
	return &metav1.APIResourceList{}
}

func isResourceAvailable(kind reflect.Type, apiResourceList *metav1.APIResourceList) bool {
	for _, apiResource := range apiResourceList.APIResources {
		// apiResource.Kind is, ex.: "PodMetrics", while kind.String() is, ex.: "*authorizationv1.PodMetrics"
		if strings.Contains(kind.String(), apiResource.Kind) {
			return true
		}
	}
	return false
}
