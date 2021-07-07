package controller

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
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

type Controller struct {
	log          logrus.FieldLogger
	clusterID    string
	castaiclient castai.Client
	provider     types.Provider
	queue        workqueue.RateLimitingInterface
	interval     time.Duration
	prepDuration time.Duration
	informers    map[reflect.Type]cache.SharedInformer

	delta        *delta
	mu           sync.Mutex
	spotCache    map[string]bool
	agentVersion *config.AgentVersion
}

func New(
	log logrus.FieldLogger,
	f informers.SharedInformerFactory,
	castaiclient castai.Client,
	provider types.Provider,
	clusterID string,
	interval time.Duration,
	prepDuration time.Duration,
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
		interval:     interval,
		prepDuration: prepDuration,
		delta:        newDelta(log, clusterID, v.Full()),
		spotCache:    map[string]bool{},
		queue:        workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "castai-agent"),
		informers:    typeInformerMap,
		agentVersion: agentVersion,
	}

	for typ, informer := range c.informers {
		typ := typ
		informer := informer
		log := log.WithField("informer", typ.String())

		var h cache.ResourceEventHandler

		if typ == reflect.TypeOf(&corev1.Node{}) {
			h = cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					c.nodeAddHandler(log, eventAdd, obj)
				},
				UpdateFunc: func(oldObj, newObj interface{}) {
					c.nodeAddHandler(log, eventUpdate, newObj)
				},
				DeleteFunc: func(obj interface{}) {
					c.nodeDeleteHandler(log, eventDelete, obj)
				},
			}
		} else {
			h = cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					genericHandler(log, c.queue, typ, eventAdd, obj)
				},
				UpdateFunc: func(oldObj, newObj interface{}) {
					genericHandler(log, c.queue, typ, eventUpdate, newObj)
				},
				DeleteFunc: func(obj interface{}) {
					genericHandler(log, c.queue, typ, eventDelete, obj)
				},
			}
		}

		informer.AddEventHandler(h)
	}

	return c
}

func (c *Controller) nodeAddHandler(log logrus.FieldLogger, event event, obj interface{}) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		log.Errorf("expected to get *corev1.Node but got %T", obj)
		return
	}

	spot, ok := c.spotCache[node.Name]
	if !ok {
		var err error
		spot, err = c.provider.IsSpot(context.Background(), node)
		if err != nil {
			log.Warnf("failed to determine whether node %q is spot: %v", node.Name, err)
		} else {
			c.spotCache[node.Name] = spot
		}
	}

	if spot {
		node.Labels[labels.FakeSpot] = "true"
	}

	genericHandler(log, c.queue, reflect.TypeOf(&corev1.Node{}), event, node)
}

func (c *Controller) nodeDeleteHandler(log logrus.FieldLogger, event event, obj interface{}) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		log.Errorf("expected to get *corev1.Node but got %T", obj)
		return
	}

	delete(c.spotCache, node.Name)

	genericHandler(log, c.queue, reflect.TypeOf(&corev1.Node{}), event, node)
}

func genericHandler(
	log logrus.FieldLogger,
	queue workqueue.RateLimitingInterface,
	expected reflect.Type,
	event event,
	obj interface{},
) {
	if reflect.TypeOf(obj) != expected {
		log.Errorf("expected to get %v but got %T", expected, obj)
		return
	}

	queue.Add(&item{
		obj:   obj.(runtime.Object),
		event: event,
	})
}

func (c *Controller) Run(ctx context.Context) {
	defer c.queue.ShutDown()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	syncs := make([]cache.InformerSynced, 0, len(c.informers))
	for _, informer := range c.informers {
		syncs = append(syncs, informer.HasSynced)
	}

	if !cache.WaitForCacheSync(ctx.Done(), syncs...) {
		c.log.Errorf("failed to sync")
		return
	}

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
		c.log.Info("collecting initial cluster snapshot")
		time.Sleep(c.prepDuration)
		c.log.Infof("sending cluster deltas every %s", c.interval)
		wait.Until(func() {
			c.send(ctx)
		}, c.interval, ctx.Done())
	}()

	go func() {
		<-ctx.Done()
		c.queue.ShutDown()
	}()

	c.pollQueueUntilDone()
}

func (c *Controller) pollQueueUntilDone() {
	for {
		i, done := c.queue.Get()
		if done {
			return
		}

		di, ok := i.(*item)
		if !ok {
			c.log.Errorf("expected queue item to be of type %T but got %T", &item{}, i)
			continue
		}

		c.mu.Lock()
		c.delta.add(di)
		c.mu.Unlock()
	}
}

func (c *Controller) send(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.castaiclient.SendDelta(ctx, c.clusterID, c.delta.toCASTAIRequest()); err != nil {
		c.log.Errorf("failed sending delta: %v", err)
		return
	}

	c.delta.clear()
}
