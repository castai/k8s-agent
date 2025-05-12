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
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	appsv1 "k8s.io/api/apps/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	authorizationtypev1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	metrics_v1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"k8s.io/metrics/pkg/client/clientset/versioned"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/controller/crd"
	"castai-agent/internal/services/controller/delta"
	"castai-agent/internal/services/controller/handlers/filters"
	"castai-agent/internal/services/controller/handlers/filters/autoscalerevents"
	"castai-agent/internal/services/controller/handlers/filters/oomevents"
	"castai-agent/internal/services/controller/handlers/transformers"
	"castai-agent/internal/services/controller/handlers/transformers/annotations"
	custominformers "castai-agent/internal/services/controller/informers"
	"castai-agent/internal/services/controller/knowngv"
	"castai-agent/internal/services/memorypressure"
	"castai-agent/internal/services/metrics"
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
	// independentInformers are not handled by informerFactory.
	// Initial use-case for them are Event informers:
	// they need independent ListOptions
	independentInformers map[string]cache.SharedIndexInformer

	fullSnapshot  bool
	deltaBatcher  *delta.Batcher[*delta.Item]
	deltaCompiler *delta.Compiler[*delta.Item]

	// sendMu ensures that only one goroutine can send deltas
	sendMu sync.Mutex

	triggerRestart func()

	agentVersion    *config.AgentVersion
	healthzProvider *HealthzProvider

	conditionalInformers    []conditionalInformer
	selfSubjectAccessReview authorizationtypev1.SelfSubjectAccessReviewInterface
	lastSend                time.Time
}

type conditionalInformer struct {
	// if empty it means all namespaces
	namespace         string
	groupVersion      schema.GroupVersion
	resource          string
	kind              string
	apiType           reflect.Type
	informerFactory   func() cache.SharedIndexInformer
	permissionVerbs   []string
	isApplied         bool
	isResourceInError bool
	transformers      transformers.Transformers
}

func (i *conditionalInformer) Name() string {
	resourceString := i.groupVersion.WithResource(i.resource).String()
	if i.namespace != "" {
		return fmt.Sprintf("Namespace:%s %s", i.namespace, resourceString)
	}
	return resourceString
}

func CollectSingleSnapshot(ctx context.Context,
	log logrus.FieldLogger,
	clusterID string,
	clientset kubernetes.Interface,
	dynamicClient dynamic.Interface,
	metricsClient versioned.Interface,
	cfg *config.Controller,
	v version.Interface,
	castwareNamespace string,
) (*castai.Delta, error) {
	tweakListOptions := func(options *metav1.ListOptions) {
		if cfg.ForcePagination && options.ResourceVersion == "0" {
			log.Info("Forcing pagination for the list request", "limit", cfg.PageSize)
			options.ResourceVersion = ""
			options.Limit = cfg.PageSize
		}
	}
	f := informers.NewSharedInformerFactoryWithOptions(clientset, 0, informers.WithTweakListOptions(tweakListOptions))
	df := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, 0, metav1.NamespaceAll, tweakListOptions)

	defaultInformers := getDefaultInformers(f, castwareNamespace, cfg.FilterEmptyReplicaSets)
	conditionalInformers := getConditionalInformers(clientset, cfg, f, df, metricsClient, log, tweakListOptions)
	additionalTransformers := createAdditionalTransformers(cfg)

	informerContext, informerCancel := context.WithCancel(ctx)
	defer informerCancel()
	queue := workqueue.NewNamed("castai-agent")
	f.Start(informerContext.Done())
	_, err, handledConditionalInformers := startConditionalInformers(informerContext, log, conditionalInformers, clientset.Discovery(), queue, clientset.AuthorizationV1().SelfSubjectAccessReviews(), additionalTransformers)
	if err != nil {
		return nil, err
	}

	handledInformers := map[string]*custominformers.HandledInformer{}
	for typ, i := range defaultInformers {
		transformers := append(i.transformers, additionalTransformers...)
		handledInformers[typ.String()] = custominformers.NewHandledInformer(log, queue, i.informer, typ, i.filters, transformers...)
	}
	for typ, i := range handledConditionalInformers {
		handledInformers[typ] = i
	}

	err = waitInformersSync(ctx, log, handledInformers)
	if err != nil {
		return nil, err
	}

	defer queue.ShutDown()

	agentVersion := "unknown"
	if vi := config.VersionInfo; vi != nil {
		agentVersion = vi.Version
	}

	deltaBatcher := delta.DefaultBatcher(log)
	deltaCompiler := delta.DefaultCompiler(log, clusterID, v.Full(), agentVersion)
	go func() {
		for {
			i, _ := queue.Get()
			if i == nil {
				return
			}
			di, ok := i.(*delta.Item)
			if !ok {
				queue.Done(i)
				log.Errorf("expected queue item to be of type %T but got %T", &delta.Item{}, i)
				continue
			}

			deltaBatcher.Write(di)
			queue.Done(i)
		}
	}()

	err = collectInitialSnapshot(ctx, log, handledInformers, queue, cfg.PrepTimeout)
	if err != nil {
		return nil, err
	}

	deltaCompiler.Write(deltaBatcher.GetMapAndClear())
	result := deltaCompiler.AsCastDelta(true)
	deltaCompiler.Clear()

	log.Debugf("synced %d items", len(result.Items))

	return result, nil
}

func New(
	log logrus.FieldLogger,
	clientset kubernetes.Interface,
	dynamicClient dynamic.Interface,
	castaiclient castai.Client,
	metricsClient versioned.Interface,
	provider types.Provider,
	clusterID string,
	cfg *config.Controller,
	v version.Interface,
	agentVersion *config.AgentVersion,
	healthzProvider *HealthzProvider,
	selfSubjectAccessReview authorizationtypev1.SelfSubjectAccessReviewInterface,
	castwareNamespace string,
) *Controller {
	healthzProvider.Initializing()

	queue := workqueue.NewNamed("castai-agent")

	defaultResync := 0 * time.Second
	tweakListOptions := func(options *metav1.ListOptions) {
		if cfg.ForcePagination && options.ResourceVersion == "0" {
			log.Info("Forcing pagination for the list request", "limit", cfg.PageSize)
			options.ResourceVersion = ""
			options.Limit = cfg.PageSize
		}
	}
	f := informers.NewSharedInformerFactoryWithOptions(clientset, defaultResync, informers.WithTweakListOptions(tweakListOptions))
	df := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, defaultResync, metav1.NamespaceAll, tweakListOptions)
	discovery := clientset.Discovery()

	defaultInformers := getDefaultInformers(f, castwareNamespace, cfg.FilterEmptyReplicaSets)
	conditionalInformers := getConditionalInformers(clientset, cfg, f, df, metricsClient, log, tweakListOptions)
	additionalTransformers := createAdditionalTransformers(cfg)

	handledInformers := map[string]*custominformers.HandledInformer{}
	independentInformers := map[string]cache.SharedIndexInformer{}
	for typ, i := range defaultInformers {
		name := typ.String()
		if !informerEnabled(cfg, name) {
			continue
		}
		transformers := append(i.transformers, additionalTransformers...)
		handledInformers[name] = custominformers.NewHandledInformer(log, queue, i.informer, typ, i.filters, transformers...)
	}

	eventType := reflect.TypeOf(&corev1.Event{})
	autoscalerEvents := fmt.Sprintf("%s:autoscaler", eventType)
	if informerEnabled(cfg, autoscalerEvents) {
		informer := createEventInformer(clientset, defaultResync, v, func(options *metav1.ListOptions, v version.Interface) {
			tweakListOptions(options)
			autoscalerevents.ListOpts(options, v)
		})
		independentInformers[autoscalerEvents] = informer
		handledInformers[autoscalerEvents] = custominformers.NewHandledInformer(
			log,
			queue,
			informer,
			eventType,
			filters.Filters{
				{
					autoscalerevents.Filter,
				},
			},
			additionalTransformers...,
		)
	}
	oomEvents := fmt.Sprintf("%s:oom", eventType)
	if informerEnabled(cfg, oomEvents) {
		informer := createEventInformer(clientset, defaultResync, v, func(options *metav1.ListOptions, v version.Interface) {
			tweakListOptions(options)
			oomevents.ListOpts(options, v)
		})
		independentInformers[oomEvents] = informer
		handledInformers[oomEvents] = custominformers.NewHandledInformer(
			log,
			queue,
			informer,
			eventType,
			filters.Filters{
				{
					oomevents.Filter,
				},
			},
			additionalTransformers...,
		)
	}

	return &Controller{
		log:                     log,
		clusterID:               clusterID,
		castaiclient:            castaiclient,
		provider:                provider,
		cfg:                     cfg,
		fullSnapshot:            true, // First snapshot is always full.
		deltaBatcher:            delta.DefaultBatcher(log),
		deltaCompiler:           delta.DefaultCompiler(log, clusterID, v.Full(), agentVersion.Version),
		queue:                   queue,
		informers:               handledInformers,
		independentInformers:    independentInformers,
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
	g := new(errgroup.Group)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	c.triggerRestart = cancel

	err := waitInformersSync(ctx, c.log, c.informers)
	if err != nil {
		return err
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
			if cfg.Resync && !c.fullSnapshot {
				c.log.Info("restarting controller to resync data")
				c.triggerRestart()
			}
		}, dur, ctx.Done())
	}()

	g.Go(func() error {
		if err := collectInitialSnapshot(ctx, c.log, c.informers, c.queue, c.cfg.PrepTimeout); err != nil {
			const maxItems = 5
			queueContent := c.debugQueueContent(maxItems)
			log := c.log.WithField("queue_content", queueContent)
			log.Errorf("error while collecting initial snapshot: %v", err)
			c.log.Infof("restarting agent after failure to collect initial snapshot")
			c.triggerRestart()
			return err
		}

		// Since both initial snapshot collection and event handlers writes to the same delta queue add
		// some sleep to prevent sending few large deltas on initial agent startup.
		c.log.Infof("sleeping for %s before starting to send cluster deltas", c.cfg.InitialSleepDuration)
		time.Sleep(c.cfg.InitialSleepDuration)

		c.healthzProvider.Initialized()

		c.log.Infof("sending cluster deltas every %s", c.cfg.Interval)

		sendDeltas := func() {
			// Check if another goroutine is trying to send deltas.
			if !c.sendMu.TryLock() {
				// If it is, don't try to send deltas on this turn to avoid a backlog of sends.
				return
			}
			// Since Mutex.TryLock() acquires a lock on success,
			// release it immediately to allow the new sending goroutine to do its job.
			defer c.sendMu.Unlock()
			c.send(ctx)
		}
		mempr := memorypressure.MemoryPressure{
			Ctx:      ctx,
			Interval: c.cfg.MemoryPressureInterval,
			Log:      c.log,
		}
		go mempr.OnMemoryPressure(sendDeltas)
		wait.NonSlidingUntil(sendDeltas, c.cfg.Interval, ctx.Done())
		return nil
	})

	go func() {
		additionalTransformers := createAdditionalTransformers(c.cfg)
		c.startConditionalInformersWithWatcher(ctx, c.conditionalInformers, additionalTransformers)
	}()

	go func() {
		<-ctx.Done()
		c.queue.ShutDown()
	}()

	c.pollQueueUntilShutdown()

	return g.Wait()
}

func informerEnabled(cfg *config.Controller, name string) bool {
	return !lo.Contains(cfg.DisabledInformers, name)
}

func (c *Controller) startConditionalInformersWithWatcher(ctx context.Context, conditionalInformers []conditionalInformer, additionalTransformers []transformers.Transformer) {
	tryConditionalInformers := conditionalInformers

	if err := wait.PollUntilContextCancel(ctx, 2*time.Minute, true, func(ctx context.Context) (done bool, err error) {
		done, err, _ = startConditionalInformers(ctx, c.log, tryConditionalInformers, c.discovery, c.queue, c.selfSubjectAccessReview, additionalTransformers)
		return done, err
	}); err != nil && !errors.Is(err, context.Canceled) {
		c.log.Errorf("error when waiting for server resources: %v", err)
	}
}

func startConditionalInformers(ctx context.Context,
	log logrus.FieldLogger,
	conditionalInformers []conditionalInformer,
	discovery discovery.DiscoveryInterface,
	queue workqueue.Interface,
	selfSubjectAccessReview authorizationtypev1.SelfSubjectAccessReviewInterface,
	additionalTransformers []transformers.Transformer,
) (bool, error, map[string]*custominformers.HandledInformer) {
	_, apiResourceLists, err := discovery.ServerGroupsAndResources()
	if err != nil {
		log.Warnf("Error when getting server resources: %v", err.Error())
		resourcesInError := extractGroupVersionsFromApiResourceError(log, err)
		for i, informer := range conditionalInformers {
			conditionalInformers[i].isResourceInError = resourcesInError[informer.groupVersion]
		}
	}
	log.Infof("Cluster API server is available, trying to start conditional informers")
	handledInformers := make(map[string]*custominformers.HandledInformer)
	for i, informer := range conditionalInformers {
		if informer.isApplied || informer.isResourceInError {
			// reset error so we can try again
			conditionalInformers[i].isResourceInError = false
			continue
		}
		apiResourceListForGroupVersion := getAPIResourceListByGroupVersion(informer.groupVersion.String(), apiResourceLists)
		if !isResourceAvailable(informer.kind, apiResourceListForGroupVersion) {
			log.Infof("Skipping conditional informer name: %v, because API resource is not available",
				informer.Name(),
			)
			continue
		}

		if !informerHasAccess(ctx, informer, selfSubjectAccessReview, log) {
			log.Warnf("Skipping conditional informer name: %v, because required access is not available",
				informer.Name(),
			)
			continue
		}

		log.Infof("Starting conditional informer for %v", informer.Name())
		conditionalInformers[i].isApplied = true

		transformers := append(informer.transformers, additionalTransformers...)
		name := fmt.Sprintf("%s::%s", informer.groupVersion.String(), informer.kind)
		handledInformer := custominformers.NewHandledInformer(log, queue, informer.informerFactory(), informer.apiType, nil, transformers...)
		handledInformers[name] = handledInformer

		go handledInformer.Run(ctx.Done())
	}

	filterNotAppliedConditionInformers := lo.Filter(conditionalInformers, func(informer conditionalInformer, _ int) bool {
		return !informer.isApplied
	})
	if len(filterNotAppliedConditionInformers) > 0 {
		return false, nil, handledInformers
	}
	return true, nil, handledInformers
}

// collectInitialSnapshot is used to add a time buffer to collect the initial snapshot which is larger than periodic
// delta because it contains a significant portion of the Kubernetes state.
func collectInitialSnapshot(
	ctx context.Context,
	log logrus.FieldLogger,
	informers map[string]*custominformers.HandledInformer,
	queue workqueue.Interface,
	prepTimeout time.Duration,
) error {
	log.Info("collecting initial cluster snapshot")

	startedAt := time.Now()

	ctx, cancel := context.WithTimeout(ctx, prepTimeout)
	defer cancel()

	// Collect initial state from cached informers and push to deltas queue.
	for _, informer := range informers {
		for _, item := range informer.GetStore().List() {
			informer.Handler.OnAdd(item, true)
		}
	}

	cond := func() (done bool, err error) {
		queueLen := queue.Len()
		log := log.WithField("queue_length", queueLen)
		log.Debug("waiting until initial queue empty")

		if queueLen == 0 {
			log.Infof("done waiting for initial cluster snapshot collection after %v", time.Since(startedAt))
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

	c.deltaBatcher.Write(di)
}

func (c *Controller) send(ctx context.Context) {
	c.trackSendMetrics(func() {
		newBatch := c.deltaBatcher.GetMapAndClear()
		c.prepareNodesForSend(ctx, newBatch)
		c.deltaCompiler.Write(newBatch)

		castDelta := c.deltaCompiler.AsCastDelta(c.fullSnapshot)
		err := c.castaiclient.SendDelta(ctx, c.clusterID, castDelta)
		if err != nil {
			if errors.Is(err, castai.ErrInvalidContinuityToken) {
				// This indicates process is in a bad state.
				c.log.Info("restarting controller due to continuity token mismatch")
				c.triggerRestart()
				return
			}
			if !errors.Is(err, context.Canceled) {
				c.log.Errorf("failed sending delta: %v", err)
			}
			// In most cases, it's because of external issues. In case it's actually an issue with the state, we rely on
			// the health provider to eventually suicide the process. Here we only skip the tick and will upload these
			// items on the next one.
			return
		}
		c.healthzProvider.SnapshotSent()
		c.deltaCompiler.Clear()
		c.fullSnapshot = false
	})
}

func (c *Controller) trackSendMetrics(op func()) {
	startTime := time.Now().UTC()
	op()
	metrics.DeltaSendTime.Observe(time.Since(startTime).Seconds())
	if !c.lastSend.IsZero() {
		metrics.DeltaSendInterval.Observe(startTime.Sub(c.lastSend).Seconds())
	}
	c.lastSend = startTime
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

func (c *Controller) prepareNodesForSend(ctx context.Context, cache map[string]*delta.Item) {
	nodesByName := map[string]*corev1.Node{}
	var nodes []*corev1.Node

	for _, item := range cache {
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
}

func informerHasAccess(ctx context.Context, informer conditionalInformer, selfSubjectAccessReview authorizationtypev1.SelfSubjectAccessReviewInterface, log logrus.FieldLogger) bool {
	// Check if allowed to access all resources with the wildcard "*" verb
	if access := informerIsAllowedToAccessResource(ctx, informer.namespace, "*", informer, informer.groupVersion.Group, selfSubjectAccessReview, log); access.Status.Allowed {
		return true
	}

	for _, verb := range informer.permissionVerbs {
		access := informerIsAllowedToAccessResource(ctx, informer.namespace, verb, informer, informer.groupVersion.Group, selfSubjectAccessReview, log)
		if !access.Status.Allowed {
			return false
		}
	}
	return true
}

func informerIsAllowedToAccessResource(ctx context.Context, namespace, verb string, informer conditionalInformer, groupName string, selfSubjectAccessReview authorizationtypev1.SelfSubjectAccessReviewInterface, log logrus.FieldLogger) *authorizationv1.SelfSubjectAccessReview {
	access, err := selfSubjectAccessReview.Create(ctx, &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace: namespace,
				Verb:      verb,
				Group:     groupName,
				Resource:  informer.resource,
			},
		},
	}, metav1.CreateOptions{})

	if err != nil {
		log.Warnf("Error when getting server resources: %v", err.Error())
		return &authorizationv1.SelfSubjectAccessReview{}
	}
	return access
}

func waitInformersSync(ctx context.Context, log logrus.FieldLogger, informers map[string]*custominformers.HandledInformer) error {
	waitStartedAt := time.Now()
	syncs := make([]cache.InformerSynced, 0, len(informers))
	syncCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	for objType, informer := range informers {
		objType := objType
		informer := informer
		logC := throttleLog(syncCtx, log, objType, waitStartedAt, 5*time.Second)
		syncs = append(syncs, func() bool {
			hasSynced := informer.SharedInformer.HasSynced()
			// By default, `cache.WaitForCacheSync` will poll every 100ms,
			// so we deduplicate the log messages via unbuffered channel.
			select {
			case logC <- hasSynced:
			}
			return hasSynced
		})
	}

	log.Info("waiting for informers cache to sync")
	if !cache.WaitForCacheSync(ctx.Done(), syncs...) {
		log.Error("failed to sync")
		return fmt.Errorf("failed to wait for cache sync")
	}
	log.Infof("informers cache synced after %v", time.Since(waitStartedAt))
	return nil
}

func throttleLog(ctx context.Context, log logrus.FieldLogger, objType string, waitStartedAt time.Time, window time.Duration) chan bool {
	logC := make(chan bool)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case synced := <-logC:
				if !synced {
					log.Infof("Informer cache for %v has not been synced after %v", objType, time.Since(waitStartedAt))
				} else {
					log.Infof("Informer cache for %v synced after %v", objType, time.Since(waitStartedAt))
				}
				select {
				case <-time.After(window):
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return logC
}

func (c *Controller) Start(done <-chan struct{}) {
	c.informerFactory.Start(done)
	for _, informer := range c.independentInformers {
		go informer.Run(done)
	}
}
func extractGroupVersionsFromApiResourceError(log logrus.FieldLogger, err error) map[schema.GroupVersion]bool {
	cleanedString := strings.Split(err.Error(), "unable to retrieve the complete list of server APIs: ")[1]
	paths := strings.Split(cleanedString, ",")

	result := make(map[schema.GroupVersion]bool)
	for _, path := range paths {
		apiPath := strings.Split(path, ":")[0]
		gv, e := schema.ParseGroupVersion(apiPath)
		if e != nil {
			log.Errorf("Error when unmarshalling group version: %v", e)
			continue
		}
		result[gv] = true
	}
	return result
}

func getConditionalInformers(
	clientset kubernetes.Interface,
	cfg *config.Controller,
	f informers.SharedInformerFactory,
	df dynamicinformer.DynamicSharedInformerFactory,
	metricsClient versioned.Interface,
	logger logrus.FieldLogger,
	tweakListOptions func(options *metav1.ListOptions),
) []conditionalInformer {
	conditionalInformers := []conditionalInformer{
		{
			groupVersion:    policyv1.SchemeGroupVersion,
			resource:        "poddisruptionbudgets",
			kind:            "PodDisruptionBudget",
			apiType:         reflect.TypeOf(&policyv1.PodDisruptionBudget{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return f.Policy().V1().PodDisruptionBudgets().Informer()
			},
		},
		{
			groupVersion:    storagev1.SchemeGroupVersion,
			resource:        "csinodes",
			kind:            "CSINode",
			apiType:         reflect.TypeOf(&storagev1.CSINode{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return f.Storage().V1().CSINodes().Informer()
			},
		},
		{
			groupVersion:    autoscalingv1.SchemeGroupVersion,
			resource:        "horizontalpodautoscalers",
			kind:            "HorizontalPodAutoscaler",
			apiType:         reflect.TypeOf(&autoscalingv1.HorizontalPodAutoscaler{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return f.Autoscaling().V1().HorizontalPodAutoscalers().Informer()
			},
		},
		{
			groupVersion:    knowngv.KarpenterCoreV1Alpha5,
			resource:        "provisioners",
			kind:            "Provisioner",
			apiType:         reflect.TypeOf(&unstructured.Unstructured{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return df.ForResource(knowngv.KarpenterCoreV1Alpha5.WithResource("provisioners")).Informer()
			},
		},
		{
			groupVersion:    knowngv.KarpenterCoreV1Alpha5,
			resource:        "machines",
			kind:            "Machine",
			apiType:         reflect.TypeOf(&unstructured.Unstructured{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return df.ForResource(knowngv.KarpenterCoreV1Alpha5.WithResource("machines")).Informer()
			},
		},
		{
			groupVersion:    knowngv.KarpenterV1Alpha1,
			resource:        "awsnodetemplates",
			kind:            "AWSNodeTemplate",
			apiType:         reflect.TypeOf(&unstructured.Unstructured{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return df.ForResource(knowngv.KarpenterV1Alpha1.WithResource("awsnodetemplates")).Informer()
			},
		},
		{
			groupVersion:    knowngv.KarpenterCoreV1Beta1,
			resource:        "nodepools",
			kind:            "NodePool",
			apiType:         reflect.TypeOf(&unstructured.Unstructured{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return df.ForResource(knowngv.KarpenterCoreV1Beta1.WithResource("nodepools")).Informer()
			},
		},
		{
			groupVersion:    knowngv.KarpenterCoreV1Beta1,
			resource:        "nodeclaims",
			kind:            "NodeClaim",
			apiType:         reflect.TypeOf(&unstructured.Unstructured{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return df.ForResource(knowngv.KarpenterCoreV1Beta1.WithResource("nodeclaims")).Informer()
			},
		},
		{
			groupVersion:    knowngv.KarpenterV1Beta1,
			resource:        "ec2nodeclasses",
			kind:            "EC2NodeClass",
			apiType:         reflect.TypeOf(&unstructured.Unstructured{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return df.ForResource(knowngv.KarpenterV1Beta1.WithResource("ec2nodeclasses")).Informer()
			},
		},
		{
			groupVersion:    knowngv.KarpenterCoreV1,
			resource:        "nodepools",
			kind:            "NodePool",
			apiType:         reflect.TypeOf(&unstructured.Unstructured{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return df.ForResource(knowngv.KarpenterCoreV1.WithResource("nodepools")).Informer()
			},
		},
		{
			groupVersion:    knowngv.KarpenterCoreV1,
			resource:        "nodeclaims",
			kind:            "NodeClaim",
			apiType:         reflect.TypeOf(&unstructured.Unstructured{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return df.ForResource(knowngv.KarpenterCoreV1.WithResource("nodeclaims")).Informer()
			},
		},
		{
			groupVersion:    knowngv.KarpenterV1,
			resource:        "ec2nodeclasses",
			kind:            "EC2NodeClass",
			apiType:         reflect.TypeOf(&unstructured.Unstructured{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return df.ForResource(knowngv.KarpenterV1.WithResource("ec2nodeclasses")).Informer()
			},
		},
		{
			groupVersion:    datadoghqv1alpha1.GroupVersion,
			resource:        "extendeddaemonsetreplicasets",
			kind:            "ExtendedDaemonSetReplicaSet",
			apiType:         reflect.TypeOf(&datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return df.ForResource(datadoghqv1alpha1.GroupVersion.WithResource("extendeddaemonsetreplicasets")).Informer()
			},
		},
		{
			groupVersion:    metrics_v1beta1.SchemeGroupVersion,
			resource:        "pods",
			kind:            "PodMetrics",
			apiType:         reflect.TypeOf(&metrics_v1beta1.PodMetrics{}),
			permissionVerbs: []string{"get", "list"},
			informerFactory: func() cache.SharedIndexInformer {
				// This informer doesn't derive itself from the `df`, so we need
				// to pass the tweakListOptions explicitly in order for it to
				// set the options.Limit
				return custominformers.NewPodMetricsInformer(logger, metricsClient, tweakListOptions)
			},
		},
		{
			groupVersion:    argorollouts.RolloutGVR.GroupVersion(),
			resource:        argorollouts.RolloutGVR.Resource,
			kind:            "Rollout",
			apiType:         reflect.TypeOf(&argorollouts.Rollout{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return df.ForResource(argorollouts.RolloutGVR).Informer()
			},
		},
		{
			groupVersion:    crd.RecommendationGVR.GroupVersion(),
			resource:        crd.RecommendationGVR.Resource,
			kind:            "Recommendation",
			apiType:         reflect.TypeOf(&crd.Recommendation{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return df.ForResource(crd.RecommendationGVR).Informer()
			},
		},
		{
			groupVersion:    networkingv1.SchemeGroupVersion,
			resource:        "ingresses",
			kind:            "Ingress",
			apiType:         reflect.TypeOf(&networkingv1.Ingress{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return f.Networking().V1().Ingresses().Informer()
			},
		},
		{
			groupVersion:    networkingv1.SchemeGroupVersion,
			resource:        "networkpolicies",
			kind:            "NetworkPolicy",
			apiType:         reflect.TypeOf(&networkingv1.NetworkPolicy{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return f.Networking().V1().NetworkPolicies().Informer()
			},
		},
		{
			groupVersion:    rbacv1.SchemeGroupVersion,
			resource:        "roles",
			kind:            "Role",
			apiType:         reflect.TypeOf(&rbacv1.Role{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return f.Rbac().V1().Roles().Informer()
			},
		},
		{
			groupVersion:    rbacv1.SchemeGroupVersion,
			resource:        "rolebindings",
			kind:            "RoleBinding",
			apiType:         reflect.TypeOf(&rbacv1.RoleBinding{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return f.Rbac().V1().RoleBindings().Informer()
			},
		},
		{
			groupVersion:    rbacv1.SchemeGroupVersion,
			resource:        "clusterroles",
			kind:            "ClusterRole",
			apiType:         reflect.TypeOf(&rbacv1.ClusterRole{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return f.Rbac().V1().ClusterRoles().Informer()
			},
		},
		{
			groupVersion:    rbacv1.SchemeGroupVersion,
			resource:        "clusterrolebindings",
			kind:            "ClusterRoleBinding",
			apiType:         reflect.TypeOf(&rbacv1.ClusterRoleBinding{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return f.Rbac().V1().ClusterRoleBindings().Informer()
			},
		},
		{
			groupVersion:    corev1.SchemeGroupVersion,
			resource:        "limitranges",
			kind:            "LimitRange",
			apiType:         reflect.TypeOf(&corev1.LimitRange{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return f.Core().V1().LimitRanges().Informer()
			},
		},
		{
			groupVersion:    corev1.SchemeGroupVersion,
			resource:        "resourcequotas",
			kind:            "ResourceQuota",
			apiType:         reflect.TypeOf(&corev1.ResourceQuota{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return f.Core().V1().ResourceQuotas().Informer()
			},
		},
		{
			groupVersion:    knowngv.RunbooksV1Alpha1,
			resource:        "recommendationsyncs",
			kind:            "RecommendationSync",
			apiType:         reflect.TypeOf(&unstructured.Unstructured{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				return df.ForResource(knowngv.RunbooksV1Alpha1.WithResource("recommendationsyncs")).Informer()
			},
		},
	}

	for _, cmNamespace := range cfg.ConfigMapNamespaces {
		conditionalInformers = append(conditionalInformers, conditionalInformer{
			namespace:       cmNamespace,
			groupVersion:    corev1.SchemeGroupVersion,
			resource:        "configmaps",
			kind:            "ConfigMap",
			apiType:         reflect.TypeOf(&corev1.ConfigMap{}),
			permissionVerbs: []string{"get", "list", "watch"},
			informerFactory: func() cache.SharedIndexInformer {
				namespaceScopedInformer := informers.NewSharedInformerFactoryWithOptions(clientset, 0, informers.WithNamespace(cmNamespace), informers.WithTweakListOptions(func(options *metav1.ListOptions) {
					if cfg.ForcePagination && options.ResourceVersion == "0" {
						logger.Info("Forcing pagination for the list request", "limit", cfg.PageSize)
						options.ResourceVersion = ""
						options.Limit = cfg.PageSize
					}
				}))
				return namespaceScopedInformer.Core().V1().ConfigMaps().Informer()
			},
		})
	}
	return conditionalInformers
}

type defaultInformer struct {
	informer     cache.SharedInformer
	filters      filters.Filters
	transformers transformers.Transformers
}

func getDefaultInformers(f informers.SharedInformerFactory, castwareNamespace string, filterEmptyReplicaSets bool) map[reflect.Type]defaultInformer {
	return map[reflect.Type]defaultInformer{
		reflect.TypeOf(&corev1.Node{}):                  {informer: f.Core().V1().Nodes().Informer()},
		reflect.TypeOf(&corev1.Pod{}):                   {informer: f.Core().V1().Pods().Informer()},
		reflect.TypeOf(&corev1.PersistentVolume{}):      {informer: f.Core().V1().PersistentVolumes().Informer()},
		reflect.TypeOf(&corev1.PersistentVolumeClaim{}): {informer: f.Core().V1().PersistentVolumeClaims().Informer()},
		reflect.TypeOf(&corev1.ReplicationController{}): {informer: f.Core().V1().ReplicationControllers().Informer()},
		reflect.TypeOf(&corev1.Namespace{}):             {informer: f.Core().V1().Namespaces().Informer()},
		reflect.TypeOf(&appsv1.Deployment{}):            {informer: f.Apps().V1().Deployments().Informer()},
		reflect.TypeOf(&appsv1.ReplicaSet{}): {
			informer: f.Apps().V1().ReplicaSets().Informer(),
			transformers: transformers.Transformers{
				func(e castai.EventType, obj interface{}) (castai.EventType, interface{}) {
					replicaSet, ok := obj.(*appsv1.ReplicaSet)
					if !ok || !filterEmptyReplicaSets {
						return e, obj
					}

					if e == castai.EventDelete || replicaSet.Namespace == castwareNamespace ||
						(replicaSet.Spec.Replicas != nil && *replicaSet.Spec.Replicas > 0 && replicaSet.Status.Replicas > 0) || replicaSet.OwnerReferences == nil {
						return e, obj
					}

					return castai.EventDelete, obj
				},
			},
		},
		reflect.TypeOf(&appsv1.DaemonSet{}):       {informer: f.Apps().V1().DaemonSets().Informer()},
		reflect.TypeOf(&appsv1.StatefulSet{}):     {informer: f.Apps().V1().StatefulSets().Informer()},
		reflect.TypeOf(&storagev1.StorageClass{}): {informer: f.Storage().V1().StorageClasses().Informer()},
		reflect.TypeOf(&batchv1.Job{}):            {informer: f.Batch().V1().Jobs().Informer()},
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

// createEventInformer creates a new event informer with the given list options.
// We can't use sharedInformerFactory because it would reuse the first registered ListOptions
// for all the informers.
func createEventInformer(client kubernetes.Interface, resyncPeriod time.Duration, v version.Interface, listOptions func(*metav1.ListOptions, version.Interface)) cache.SharedIndexInformer {
	return v1.NewFilteredEventInformer(client, corev1.NamespaceAll, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, func(options *metav1.ListOptions) {
		listOptions(options, v)
	})
}

func getAPIResourceListByGroupVersion(groupVersion string, apiResourceLists []*metav1.APIResourceList) *metav1.APIResourceList {
	for _, apiResourceList := range apiResourceLists {
		if apiResourceList.GroupVersion == groupVersion {
			return apiResourceList
		}
	}
	return &metav1.APIResourceList{}
}

func isResourceAvailable(kind string, apiResourceList *metav1.APIResourceList) bool {
	for _, apiResource := range apiResourceList.APIResources {
		if kind == apiResource.Kind {
			return true
		}
	}
	return false
}

func createAdditionalTransformers(cfg *config.Controller) []transformers.Transformer {
	return []transformers.Transformer{
		annotations.NewTransformer(cfg.RemoveAnnotationsPrefixes, cfg.AnnotationsMaxLength),
	}
}
