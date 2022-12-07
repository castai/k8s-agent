//go:generate mockgen -destination ./mock/workqueue.go k8s.io/client-go/util/workqueue Interface
package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sync"
	"time"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	custominformers "castai-agent/internal/services/controller/informers"
	"castai-agent/internal/services/providers/types"
	"castai-agent/internal/services/version"
	"castai-agent/pkg/labels"

	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"k8s.io/metrics/pkg/client/clientset/versioned"
)

var (
	// sensitiveValuePattern matches strings which are usually used to name variables holding sensitive values, like
	// passwords. This is a case-insensitive match of the listed words.
	sensitiveValuePattern = regexp.MustCompile(`(?i)passwd|pass|password|pwd|secret|token|key`)
)

type Controller struct {
	log          logrus.FieldLogger
	clusterID    string
	castaiclient castai.Client
	provider     types.Provider
	queue        workqueue.Interface
	cfg          *config.Controller
	informers    map[reflect.Type]cache.SharedInformer

	delta   *delta
	deltaMu sync.Mutex

	triggerRestart func()

	agentVersion    *config.AgentVersion
	healthzProvider *HealthzProvider
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

	typeInformerMap := map[reflect.Type]cache.SharedInformer{
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

	if v.MinorInt() >= 17 {
		typeInformerMap[reflect.TypeOf(&storagev1.CSINode{})] = f.Storage().V1().CSINodes().Informer()
	}

	if v.MinorInt() >= 18 {
		typeInformerMap[reflect.TypeOf(&autoscalingv1.HorizontalPodAutoscaler{})] =
			f.Autoscaling().V1().HorizontalPodAutoscalers().Informer()
	}

	_, res, err := discovery.ServerGroupsAndResources()
	if err == nil {
		supportsMetrics := lo.SomeBy(res, func(resourceList *metav1.APIResourceList) bool {
			if resourceList.GroupVersion != "metrics.k8s.io/v1beta1" {
				return false
			}

			return lo.SomeBy(resourceList.APIResources, func(resource metav1.APIResource) bool {
				return resource.Kind == "PodMetrics"
			})
		})

		if supportsMetrics {
			log.Infof("Cluster supports pod metrics, will collect them.")
			metricsInformer := f.InformerFor(&v1beta1.PodMetrics{}, func(_ kubernetes.Interface, _ time.Duration) cache.SharedIndexInformer {
				return custominformers.NewPodMetricsInformer(log, metricsClient)
			})

			typeInformerMap[reflect.TypeOf(&v1beta1.PodMetrics{})] = metricsInformer
		}
	} else {
		log.Errorf("Failed to discover API Resources")
	}

	c := &Controller{
		log:             log,
		clusterID:       clusterID,
		castaiclient:    castaiclient,
		provider:        provider,
		cfg:             cfg,
		delta:           newDelta(log, clusterID, v.Full()),
		queue:           workqueue.NewNamed("castai-agent"),
		informers:       typeInformerMap,
		agentVersion:    agentVersion,
		healthzProvider: healthzProvider,
	}

	c.registerEventHandlers()

	return c
}

func (c *Controller) registerEventHandlers() {
	for typ, informer := range c.informers {
		typ := typ
		informer := informer
		log := c.log.WithField("informer", typ.String())
		h := c.createEventHandlers(log, typ)
		informer.AddEventHandler(h)
	}
}

func (c *Controller) createEventHandlers(log logrus.FieldLogger, typ reflect.Type) cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			c.deletedUnknownHandler(log, eventAdd, obj, func(log logrus.FieldLogger, e event, obj interface{}) {
				c.genericHandler(log, typ, e, obj)
			})
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			c.deletedUnknownHandler(log, eventUpdate, newObj, func(log logrus.FieldLogger, e event, obj interface{}) {
				c.genericHandler(log, typ, e, obj)
			})
		},
		DeleteFunc: func(obj interface{}) {
			c.deletedUnknownHandler(log, eventDelete, obj, func(log logrus.FieldLogger, e event, obj interface{}) {
				c.genericHandler(log, typ, e, obj)
			})
		},
	}
}

type handlerFunc func(log logrus.FieldLogger, event event, obj interface{})

// deletedUnknownHandler is used to handle cache.DeletedFinalStateUnknown where an object was deleted but the watch
// deletion event was missed while disconnected from the api-server.
func (c *Controller) deletedUnknownHandler(log logrus.FieldLogger, e event, obj interface{}, next handlerFunc) {
	if deleted, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		next(log, eventDelete, deleted.Obj)
	} else {
		next(log, e, obj)
	}
}

// genericHandler is used to add an object to the queue.
func (c *Controller) genericHandler(
	log logrus.FieldLogger,
	expected reflect.Type,
	e event,
	obj interface{},
) {
	if reflect.TypeOf(obj) != expected {
		log.Errorf("expected to get %v but got %T", expected, obj)
		return
	}

	cleanObj(obj)

	c.log.Debugf("generic handler called: %s: %s", e, reflect.TypeOf(obj))

	c.queue.Add(&item{
		obj:   obj.(object),
		event: e,
	})
}

// cleanObj removes unnecessary or sensitive values from K8s objects.
func cleanObj(obj interface{}) {
	removeManagedFields(obj)
	removeSensitiveEnvVars(obj)
}

func removeManagedFields(obj interface{}) {
	if metaobj, ok := obj.(metav1.Object); ok {
		metaobj.SetManagedFields(nil)
	}
}

func removeSensitiveEnvVars(obj interface{}) {
	var containers []*corev1.Container

	switch o := obj.(type) {
	case *corev1.Pod:
		for i := range o.Spec.Containers {
			containers = append(containers, &o.Spec.Containers[i])
		}
	case *appsv1.Deployment:
		for i := range o.Spec.Template.Spec.Containers {
			containers = append(containers, &o.Spec.Template.Spec.Containers[i])
		}
	case *appsv1.StatefulSet:
		for i := range o.Spec.Template.Spec.Containers {
			containers = append(containers, &o.Spec.Template.Spec.Containers[i])
		}
	case *appsv1.ReplicaSet:
		for i := range o.Spec.Template.Spec.Containers {
			containers = append(containers, &o.Spec.Template.Spec.Containers[i])
		}
	case *appsv1.DaemonSet:
		for i := range o.Spec.Template.Spec.Containers {
			containers = append(containers, &o.Spec.Template.Spec.Containers[i])
		}
	}

	if len(containers) == 0 {
		return
	}

	for _, c := range containers {
		c.Env = lo.Filter(c.Env, func(envVar corev1.EnvVar, _ int) bool {
			return envVar.Value == "" || !sensitiveValuePattern.MatchString(envVar.Name)
		})
	}
}

func (c *Controller) Run(ctx context.Context) error {
	defer c.queue.ShutDown()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	c.triggerRestart = cancel

	syncs := make([]cache.InformerSynced, 0, len(c.informers))
	for _, informer := range c.informers {
		syncs = append(syncs, informer.HasSynced)
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
			if cfg.Resync && !c.delta.fullSnapshot {
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

	go func() {
		<-ctx.Done()
		c.queue.ShutDown()
	}()

	c.pollQueueUntilShutdown()

	return nil
}

// collectInitialSnapshot is used to add a time buffer to collect the initial snapshot which is larger than periodic
// delta because it contains a significant portion of the Kubernetes state.
func (c *Controller) collectInitialSnapshot(ctx context.Context) error {
	c.log.Info("collecting initial cluster snapshot")

	startedAt := time.Now()

	ctx, cancel := context.WithTimeout(ctx, c.cfg.PrepTimeout)
	defer cancel()

	// Collect initial state from cached informers and push to deltas queue.
	for objType, informer := range c.informers {
		for _, item := range informer.GetStore().List() {
			c.genericHandler(c.log, objType, eventAdd, item)
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

	di, ok := i.(*item)
	if !ok {
		c.log.Errorf("expected queue item to be of type %T but got %T", &item{}, i)
		return
	}

	c.deltaMu.Lock()
	c.delta.add(di)
	c.deltaMu.Unlock()
}

func (c *Controller) send(ctx context.Context) {
	c.deltaMu.Lock()
	defer c.deltaMu.Unlock()

	nodesByName := map[string]*corev1.Node{}
	var nodes []*corev1.Node

	for _, item := range c.delta.cache {
		n, ok := item.obj.(*corev1.Node)
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

	if err := c.castaiclient.SendDelta(ctx, c.clusterID, c.delta.toCASTAIRequest()); err != nil {
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

	c.delta.clear()
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
