//go:generate mockgen -destination ./mock/workqueue.go k8s.io/client-go/util/workqueue Interface
package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/providers/types"
	"castai-agent/internal/services/version"
	"castai-agent/pkg/labels"
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

	agentVersion *config.AgentVersion
}

func New(
	log logrus.FieldLogger,
	f informers.SharedInformerFactory,
	castaiclient castai.Client,
	provider types.Provider,
	clusterID string,
	cfg *config.Controller,
	v version.Interface,
	agentVersion *config.AgentVersion,
) *Controller {
	typeInformerMap := map[reflect.Type]cache.SharedInformer{
		reflect.TypeOf(&corev1.Node{}):                  f.Core().V1().Nodes().Informer(),
		reflect.TypeOf(&corev1.Pod{}):                   f.Core().V1().Pods().Informer(),
		reflect.TypeOf(&corev1.PersistentVolume{}):      f.Core().V1().PersistentVolumes().Informer(),
		reflect.TypeOf(&corev1.PersistentVolumeClaim{}): f.Core().V1().PersistentVolumeClaims().Informer(),
		reflect.TypeOf(&corev1.ReplicationController{}): f.Core().V1().ReplicationControllers().Informer(),
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

	c := &Controller{
		log:          log,
		clusterID:    clusterID,
		castaiclient: castaiclient,
		provider:     provider,
		cfg:          cfg,
		delta:        newDelta(log, clusterID, v.Full()),
		queue:        workqueue.NewNamed("castai-agent"),
		informers:    typeInformerMap,
		agentVersion: agentVersion,
	}

	c.registerEventHandlers()

	return c
}

var eventsDumpPath = fmt.Sprintf("/tmp/events-%d.jsonlines", time.Now().Unix())

func (c *Controller) registerEventHandlers() {
	for typ, informer := range c.informers {
		typ := typ
		informer := informer
		log := c.log.WithField("informer", typ.String())
		h := c.createEventHandlers(log, typ)
		if config.Debug {
			sink, _, err := wrapWithFileSink(eventsDumpPath, log, h)
			if err != nil {
				log.Warnf("failed to wrap informer: %v", err)
			} else {
				h = sink
			}
		}
		informer.AddEventHandler(h)
	}
}

func (c *Controller) createEventHandlers(log logrus.FieldLogger, typ reflect.Type) cache.ResourceEventHandler {
	if typ == reflect.TypeOf(&corev1.Node{}) {
		return c.createNodeEventHandlers(log, typ)
	}
	return c.createGenericEventHandlers(log, typ)
}

func (c *Controller) createNodeEventHandlers(log logrus.FieldLogger, typ reflect.Type) cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			c.deletedUnknownHandler(log, eventAdd, obj, func(log logrus.FieldLogger, e event, obj interface{}) {
				c.nodeAddHandler(log, e, obj, func(log logrus.FieldLogger, e event, obj interface{}) {
					c.genericHandler(log, typ, e, obj)
				})
			})
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			c.deletedUnknownHandler(log, eventUpdate, newObj, func(log logrus.FieldLogger, e event, obj interface{}) {
				c.nodeAddHandler(log, e, obj, func(log logrus.FieldLogger, e event, obj interface{}) {
					c.genericHandler(log, typ, e, obj)
				})
			})
		},
		DeleteFunc: func(obj interface{}) {
			c.deletedUnknownHandler(log, eventDelete, obj, func(log logrus.FieldLogger, e event, obj interface{}) {
				c.nodeDeleteHandler(log, e, obj, func(log logrus.FieldLogger, e event, obj interface{}) {
					c.genericHandler(log, typ, e, obj)
				})
			})
		},
	}
}

func (c *Controller) createGenericEventHandlers(log logrus.FieldLogger, typ reflect.Type) cache.ResourceEventHandler {
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

// nodeAddHandler is used to handle only node events to carry out some node-only logic, like figuring out
// whether the node is a spot instance.
func (c *Controller) nodeAddHandler(log logrus.FieldLogger, e event, obj interface{}, next handlerFunc) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		log.Errorf("expected to get *corev1.Node but got %T", obj)
		return
	}

	var err error
	spot, err := c.provider.IsSpot(context.Background(), node)
	if err != nil {
		log.Warnf("failed to determine whether node %q is spot: %v", node.Name, err)
	}

	if spot {
		node.Labels[labels.CastaiFakeSpot] = "true"
	}

	next(log, e, node)
}

// nodeDeleteHandler is used to handle only node events to carry out some node-only logic, like clearing out the
// spot cache.
func (c *Controller) nodeDeleteHandler(log logrus.FieldLogger, e event, obj interface{}, next handlerFunc) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		log.Errorf("expected to get *corev1.Node but got %T", obj)
		return
	}

	next(log, e, node)
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

	c.log.WithField("queue_size", c.queue.Len()).Debugf("generic handler called: %s: %s", e, reflect.TypeOf(obj))

	c.queue.Add(&item{
		obj:   obj.(runtime.Object),
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

	isSensitiveEnvVar := func(envVar corev1.EnvVar) bool {
		if envVar.Value == "" {
			return false
		}
		return sensitiveValuePattern.MatchString(envVar.Name)
	}

	for _, c := range containers {
		validIdx := 0
		for _, envVar := range c.Env {
			if isSensitiveEnvVar(envVar) {
				continue
			}
			c.Env[validIdx] = envVar
			validIdx++
		}
		c.Env = c.Env[:validIdx]
	}
}

func (c *Controller) Run(ctx context.Context) {
	defer c.queue.ShutDown()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	syncs := make([]cache.InformerSynced, 0, len(c.informers))
	for _, informer := range c.informers {
		syncs = append(syncs, informer.HasSynced)
	}

	waitStartedAt := time.Now()
	c.log.Info("waiting for informers cache to sync")
	if !cache.WaitForCacheSync(ctx.Done(), syncs...) {
		c.log.Error("failed to sync")
		return
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
				cancel()
			}
		}, dur, ctx.Done())
	}()

	go func() {
		if err := c.collectInitialSnapshot(ctx); err != nil {
			const maxItems = 5
			queueContent := c.debugQueueContent(maxItems)
			log := c.log.WithField("queue_content", queueContent)
			if config.Debug {
				log.Errorf("error while collecting initial snapshot: %v", err)
				log.Warnf("agent will now sleep forver. Dumped events available at: %q", eventsDumpPath)
			}
			// Crash agent in case it's not able to collect full snapshot from informers cache.
			log.Fatalf("error while collecting initial snapshot: %v", err)
		}

		// Since both initial snapshot collection and event handlers writes to the same delta queue add
		// some sleep to prevent sending few large deltas on initial agent startup.
		c.log.Infof("sleeping for %s before starting to send cluster deltas", c.cfg.InitialSleepDuration)
		time.Sleep(c.cfg.InitialSleepDuration)

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
}

// collectInitialSnapshot is used to add a time buffer to collect the initial snapshot which is larger than periodic
// delta because it contains a significant portion of the Kubernetes state.
func (c *Controller) collectInitialSnapshot(ctx context.Context) error {
	timeout := c.cfg.PrepTimeout
	c.log.Infof("collecting initial cluster snapshot with timeout of %v", timeout)

	startedAt := time.Now()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Collect initial state from cached informers and push to deltas queue.
	for objType, informer := range c.informers {
		c.log.Debugf("processing informer type %v", objType)
		items := informer.GetStore().List()
		switch objType {
		case reflect.TypeOf(&corev1.Node{}):
			for _, item := range items {
				c.log.Debugf("processing node type %v", item)
				c.nodeAddHandler(c.log, eventAdd, item, func(log logrus.FieldLogger, event event, obj interface{}) {
					c.genericHandler(log, objType, event, obj)
				})
			}
		default:
			for _, item := range items {
				c.log.Debugf("processing generic type %v", item)
				c.genericHandler(c.log, objType, eventAdd, item)
			}
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

	if err := c.castaiclient.SendDelta(ctx, c.clusterID, c.delta.toCASTAIRequest()); err != nil {
		c.log.Errorf("failed sending delta: %v", err)
		return
	}

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
