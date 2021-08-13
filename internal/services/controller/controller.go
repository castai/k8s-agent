package controller

import (
	"context"
	"errors"
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
		queue:        workqueue.NewNamed("castai-agent"),
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
	queue workqueue.Interface,
	expected reflect.Type,
	event event,
	obj interface{},
) {
	if reflect.TypeOf(obj) != expected {
		log.Errorf("expected to get %v but got %T", expected, obj)
		return
	}

	cleanObj(obj)

	queue.Add(&item{
		obj:   obj.(runtime.Object),
		event: event,
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
		if err := c.collectInitialSnapshot(ctx); err != nil {
			c.log.Errorf("error while collecting initial snapshot: %v", err)
		}

		c.log.Infof("sending cluster deltas every %s", c.interval)
		wait.Until(func() {
			c.send(ctx)
		}, c.interval, ctx.Done())
	}()

	go func() {
		<-ctx.Done()
		c.queue.ShutDown()
	}()

	c.pollQueueUntilShutdown()
}

// collectInitialSnapshot is used to add a time buffer to collect the initial snapshot which is larger than periodic
// delta because it contains a significant portion of the Kubernetes state. The function has multiple exit points:
// 	1. Exits when the workqueue.Interface reports queue length of 0.
//	2. The deadline of prepDuration has expired.
func (c *Controller) collectInitialSnapshot(ctx context.Context) error {
	c.log.Info("collecting initial cluster snapshot")
	start := time.Now().UTC()
	deadline := start.Add(c.prepDuration)

	cond := func() (done bool, err error) {
		defer func() {
			if !done {
				return
			}
			c.log.Infof("done waiting for initial cluster snapshot collection after %v", time.Now().UTC().Sub(start))
		}()

		queueLen := c.queue.Len()
		log := c.log.WithField("queue_length", queueLen)
		log.Debug("waiting until initial queue empty")

		if queueLen == 0 {
			log.Debug("initial workqueue is empty")
			return true, nil
		}

		if time.Now().UTC().After(deadline) {
			log.Debug("initial cluster snapshot collection deadline reached")
			return true, nil
		}

		return false, nil
	}

	if err := wait.PollImmediateUntil(time.Second, cond, ctx.Done()); err != nil && !errors.Is(err, wait.ErrWaitTimeout) {
		return err
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

	c.mu.Lock()
	c.delta.add(di)
	c.mu.Unlock()
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
