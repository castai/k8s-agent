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

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	argorollouts "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	karpenterCore "github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	karpenter "github.com/aws/karpenter/pkg/apis/v1alpha1"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	authorizationtypev1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
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
	informers    map[string]*custominformers.HandledInformer

	discovery       discovery.DiscoveryInterface
	metricsClient   versioned.Interface
	informerFactory informers.SharedInformerFactory

	delta   *delta.Delta
	deltaMu sync.Mutex

	triggerRestart func()

	agentVersion    *config.AgentVersion
	healthzProvider *HealthzProvider

	conditionalInformers    []conditionalInformer
	selfSubjectAccessReview authorizationtypev1.SelfSubjectAccessReviewInterface
}

type conditionalInformer struct {
	resource        schema.GroupVersionResource
	apiType         reflect.Type
	informerFactory func() cache.SharedIndexInformer
	permissionVerbs []string
	isApplied       bool
}

func New(
	log logrus.FieldLogger,
	f informers.SharedInformerFactory,
	df dynamicinformer.DynamicSharedInformerFactory,
	discovery discovery.DiscoveryInterface,
	castaiclient castai.Client,
	metricsClient versioned.Interface,
	provider types.Provider,
	clusterID string,
	cfg *config.Controller,
	v version.Interface,
	agentVersion *config.AgentVersion,
	healthzProvider *HealthzProvider,
	selfSubjectAccessReview authorizationtypev1.SelfSubjectAccessReviewInterface,
) *Controller {
	healthzProvider.Initializing()

	queue := workqueue.NewNamed("castai-agent")

	defaultInformers := getDefaultInformers(f)
	conditionalInformers := getConditionalInformers(f, df, metricsClient, log)

	handledInformers := map[string]*custominformers.HandledInformer{}
	for typ, i := range defaultInformers {
		handledInformers[typ.String()] = custominformers.NewHandledInformer(log, queue, i.informer, typ, i.filters)
	}

	eventType := reflect.TypeOf(&corev1.Event{})
	handledInformers[fmt.Sprintf("%s:autoscaler", eventType)] = custominformers.NewHandledInformer(
		log,
		queue,
		createEventInformer(f, v, autoscalerevents.ListOpts),
		eventType,
		filters.Filters{
			{
				autoscalerevents.Filter,
			},
		},
	)
	handledInformers[fmt.Sprintf("%s:oom", eventType)] = custominformers.NewHandledInformer(
		log,
		queue,
		createEventInformer(f, v, oomevents.ListOpts),
		eventType,
		filters.Filters{
			{
				oomevents.Filter,
			},
		},
	)

	return &Controller{
		log:                     log,
		clusterID:               clusterID,
		castaiclient:            castaiclient,
		provider:                provider,
		cfg:                     cfg,
		delta:                   delta.New(log, clusterID, v.Full()),
		queue:                   queue,
		informers:               handledInformers,
		agentVersion:            agentVersion,
		healthzProvider:         healthzProvider,
		metricsClient:           metricsClient,
		discovery:               discovery,
		informerFactory:         f,
		conditionalInformers:    conditionalInformers,
		selfSubjectAccessReview: selfSubjectAccessReview,
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
				c.log.Infof("Informer cache for %v has not been synced.", objType)
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

	go func() {
		<-ctx.Done()
		c.queue.ShutDown()
	}()

	c.pollQueueUntilShutdown()

	return nil
}

func (c *Controller) startConditionalInformersWithWatcher(ctx context.Context, conditionalInformers []conditionalInformer) {
	tryConditionalInformers := conditionalInformers

	if err := wait.PollUntilContextCancel(ctx, 2*time.Minute, true, func(ctx context.Context) (done bool, err error) {
		apiResourceLists := fetchAPIResourceLists(c.discovery, c.log)
		if apiResourceLists == nil {
			return false, nil
		}
		c.log.Infof("Cluster API server is available, trying to start conditional informers")

		for i, informer := range tryConditionalInformers {
			if informer.isApplied {
				continue
			}
			apiResourceListForGroupVersion := getAPIResourceListByGroupVersion(informer.resource.GroupVersion().String(), apiResourceLists)
			if !isResourceAvailable(informer.apiType, apiResourceListForGroupVersion) {
				c.log.Warnf("Skipping conditional informer name: %v, because API resource is not available",
					informer.resource.String(),
				)
				continue
			}

			if !c.informerHasAccess(ctx, informer) {
				c.log.Warnf("Skipping conditional informer name: %v, because required access is not available",
					informer.resource.String(),
				)
				continue
			}

			c.log.Infof("Starting conditional informer for %v", informer.resource.String())
			tryConditionalInformers[i].isApplied = true

			handledInformer := custominformers.NewHandledInformer(c.log, c.queue, informer.informerFactory(), informer.apiType, nil)

			go handledInformer.Run(ctx.Done())
		}

		filterNotAppliedConditionInformers := lo.Filter(tryConditionalInformers, func(informer conditionalInformer, _ int) bool {
			return !informer.isApplied
		})
		if len(filterNotAppliedConditionInformers) > 0 {
			return false, nil
		}
		return true, nil
	}); err != nil && !errors.Is(err, context.Canceled) {
		c.log.Errorf("error when waiting for server resources: %v", err)
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
			informer.Handler.OnAdd(item, true)
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

func (c *Controller) informerHasAccess(ctx context.Context, informer conditionalInformer) bool {
	// Check if allowed to access all resources with the wildcard "*" verb
	if access := c.informerIsAllowedToAccessResource(ctx, "*", informer, informer.resource.Group); access.Status.Allowed {
		return true
	}

	for _, verb := range informer.permissionVerbs {
		access := c.informerIsAllowedToAccessResource(ctx, verb, informer, informer.resource.Group)
		if !access.Status.Allowed {
			return false
		}
	}
	return true
}

func (c *Controller) informerIsAllowedToAccessResource(ctx context.Context, verb string, informer conditionalInformer, groupName string) *authorizationv1.SelfSubjectAccessReview {
	access, err := c.selfSubjectAccessReview.Create(ctx, &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Verb:     verb,
				Group:    groupName,
				Resource: informer.resource.Resource,
			},
		},
	}, metav1.CreateOptions{})

	if err != nil {
		c.log.Warnf("Error when getting server resources: %v", err.Error())
		return &authorizationv1.SelfSubjectAccessReview{}
	}
	return access
}

func getConditionalInformers(f informers.SharedInformerFactory, df dynamicinformer.DynamicSharedInformerFactory, metricsClient versioned.Interface, logger logrus.FieldLogger) []conditionalInformer {
	return []conditionalInformer{
		{
			resource:        corev1.SchemeGroupVersion.WithResource("configmaps"),
			apiType:         reflect.TypeOf(&corev1.ConfigMap{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return f.Core().V1().ConfigMaps().Informer()
			},
		},
		{
			resource:        policyv1.SchemeGroupVersion.WithResource("poddisruptionbudgets"),
			apiType:         reflect.TypeOf(&policyv1.PodDisruptionBudget{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return f.Policy().V1().PodDisruptionBudgets().Informer()
			},
		},
		{
			resource:        storagev1.SchemeGroupVersion.WithResource("csinodes"),
			apiType:         reflect.TypeOf(&storagev1.CSINode{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return f.Storage().V1().CSINodes().Informer()
			},
		},
		{
			resource:        autoscalingv1.SchemeGroupVersion.WithResource("horizontalpodautoscalers"),
			apiType:         reflect.TypeOf(&autoscalingv1.HorizontalPodAutoscaler{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return f.Autoscaling().V1().HorizontalPodAutoscalers().Informer()
			},
		},
		{
			resource:        karpenterCore.SchemeGroupVersion.WithResource("provisioners"),
			apiType:         reflect.TypeOf(&karpenterCore.Provisioner{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return df.ForResource(karpenterCore.SchemeGroupVersion.WithResource("provisioners")).Informer()
			},
		},
		{
			resource:        karpenterCore.SchemeGroupVersion.WithResource("machines"),
			apiType:         reflect.TypeOf(&karpenterCore.Machine{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return df.ForResource(karpenterCore.SchemeGroupVersion.WithResource("machines")).Informer()
			},
		},
		{
			resource:        karpenter.SchemeGroupVersion.WithResource("awsnodetemplates"),
			apiType:         reflect.TypeOf(&karpenter.AWSNodeTemplate{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return df.ForResource(karpenter.SchemeGroupVersion.WithResource("awsnodetemplates")).Informer()
			},
		},
		{
			resource:        datadoghqv1alpha1.GroupVersion.WithResource("extendeddaemonsetreplicasets"),
			apiType:         reflect.TypeOf(&datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return df.ForResource(datadoghqv1alpha1.GroupVersion.WithResource("extendeddaemonsetreplicasets")).Informer()
			},
		},
		{
			resource:        v1beta1.SchemeGroupVersion.WithResource("pods"),
			apiType:         reflect.TypeOf(&v1beta1.PodMetrics{}),
			permissionVerbs: []string{"get", "list"},
			informerFactory: func() cache.SharedIndexInformer {
				return custominformers.NewPodMetricsInformer(logger, metricsClient)
			},
		},
		{
			resource:        argorollouts.RolloutGVR,
			apiType:         reflect.TypeOf(&argorollouts.Rollout{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return df.ForResource(argorollouts.RolloutGVR).Informer()
			},
		},
	}
}

type defaultInformer struct {
	informer cache.SharedInformer
	filters  filters.Filters
}

func getDefaultInformers(f informers.SharedInformerFactory) map[reflect.Type]defaultInformer {
	return map[reflect.Type]defaultInformer{
		reflect.TypeOf(&corev1.Node{}):                  {informer: f.Core().V1().Nodes().Informer()},
		reflect.TypeOf(&corev1.Pod{}):                   {informer: f.Core().V1().Pods().Informer()},
		reflect.TypeOf(&corev1.PersistentVolume{}):      {informer: f.Core().V1().PersistentVolumes().Informer()},
		reflect.TypeOf(&corev1.PersistentVolumeClaim{}): {informer: f.Core().V1().PersistentVolumeClaims().Informer()},
		reflect.TypeOf(&corev1.ReplicationController{}): {informer: f.Core().V1().ReplicationControllers().Informer()},
		reflect.TypeOf(&corev1.Namespace{}):             {informer: f.Core().V1().Namespaces().Informer()},
		reflect.TypeOf(&appsv1.Deployment{}):            {informer: f.Apps().V1().Deployments().Informer()},
		reflect.TypeOf(&appsv1.ReplicaSet{}):            {informer: f.Apps().V1().ReplicaSets().Informer()},
		reflect.TypeOf(&appsv1.DaemonSet{}):             {informer: f.Apps().V1().DaemonSets().Informer()},
		reflect.TypeOf(&appsv1.StatefulSet{}):           {informer: f.Apps().V1().StatefulSets().Informer()},
		reflect.TypeOf(&storagev1.StorageClass{}):       {informer: f.Storage().V1().StorageClasses().Informer()},
		reflect.TypeOf(&batchv1.Job{}):                  {informer: f.Batch().V1().Jobs().Informer()},
		reflect.TypeOf(&corev1.Service{}): {
			informer: f.Core().V1().Services().Informer(),
			filters: filters.Filters{
				{
					// spec.type isn't supported as a field selector, so we need to filter it out locally
					func(e castai.EventType, obj interface{}) bool {
						svc, ok := obj.(*corev1.Service)
						if !ok {
							return false
						}
						return svc.Spec.Type != corev1.ServiceTypeExternalName
					},
				},
			},
		},
	}
}

func createEventInformer(f informers.SharedInformerFactory, v version.Interface, listOptions func(*metav1.ListOptions, version.Interface)) cache.SharedIndexInformer {
	return f.InformerFor(&corev1.Event{}, func(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
		return v1.NewFilteredEventInformer(client, corev1.NamespaceAll, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, func(options *metav1.ListOptions) {
			listOptions(options, v)
		})
	})
}

func fetchAPIResourceLists(client discovery.DiscoveryInterface, log logrus.FieldLogger) []*metav1.APIResourceList {
	_, apiResourceLists, err := client.ServerGroupsAndResources()
	if err != nil {
		log.Warnf("Error when getting server resources: %v", err.Error())
		return nil
	}
	return apiResourceLists
}

func getAPIResourceListByGroupVersion(groupVersion string, apiResourceLists []*metav1.APIResourceList) *metav1.APIResourceList {
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
		// apiResource.Kind is, ex.: "PodMetrics", while kind.String() is, ex.: "*v1.PodMetrics"
		if strings.Contains(kind.String(), apiResource.Kind) {
			return true
		}
	}
	return false
}
