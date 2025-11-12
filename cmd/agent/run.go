package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/component-base/metrics/legacyregistry"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"k8s.io/metrics/pkg/client/clientset/versioned"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"castai-agent/pkg/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/controller"
	"castai-agent/internal/services/controller/scheme"
	"castai-agent/internal/services/discovery"
	"castai-agent/internal/services/health"
	"castai-agent/internal/services/metadata"
	"castai-agent/internal/services/metrics"
	"castai-agent/internal/services/monitor"
	"castai-agent/pkg/services/providers"
	"castai-agent/internal/services/replicas"
	castailog "castai-agent/pkg/log"
)

const (
	exitChannelBufferSize = 10
	leaderHealthzTimeout  = 2 * time.Minute
	bytesInMiB            = 1024 * 1024
)

func run(ctx context.Context) error {
	cfg := config.Get()
	if cfg.API.Key == "" {
		return errors.New("env variable \"API_KEY\" is required")
	}
	if cfg.API.URL == "" {
		return errors.New("env variable \"API_URL\" is required")
	}

	textFormatter := logrus.TextFormatter{FullTimestamp: true}
	remoteLogger := logrus.New()
	remoteLogger.SetLevel(logrus.Level(cfg.Log.Level))
	remoteLogger.SetFormatter(&textFormatter)
	log := remoteLogger.WithField("version", config.VersionInfo.Version)
	if podName := os.Getenv("SELF_POD_NAME"); podName != "" {
		log = log.WithField("component_pod_name", podName)
	}

	if nodeName := os.Getenv("SELF_POD_NODE"); nodeName != "" {
		log = log.WithField("component_node_name", nodeName)
	}

	localLog := logrus.New()
	localLog.SetLevel(logrus.DebugLevel)

	loggingConfig := castailog.Config{
		SendTimeout: cfg.Log.ExporterSenderTimeout,
	}

	if cfg.Log.PrintMemoryUsageEvery != nil {
		go func() {
			ticker := time.NewTicker(*cfg.Log.PrintMemoryUsageEvery)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					var m runtime.MemStats
					runtime.ReadMemStats(&m)
					log.WithFields(logrus.Fields{
						"Alloc_MiB":      m.Alloc / bytesInMiB,
						"TotalAlloc_MiB": m.TotalAlloc / bytesInMiB,
						"Sys_MiB":        m.Sys / bytesInMiB,
						"NumGC":          m.NumGC,
					}).Info("memory usage")
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	restyClient, err := castai.NewDefaultRestyClient()
	if err != nil {
		log.Fatalf("resty client failed: %v", err)
	}

	deltaHTTPClient, err := castai.NewDefaultDeltaHTTPClient()
	if err != nil {
		log.Fatalf("delta http client failed: %v", err)
	}

	castaiClient := castai.NewClient(log, restyClient, deltaHTTPClient)

	registrator := castai.NewRegistrator()
	defer registrator.ReleaseWaiters()

	castailog.SetupLogExporter(registrator, remoteLogger, localLog, castaiClient, &loggingConfig)

	clusterIDHandler := func(clusterID string) {
		loggingConfig.ClusterID = clusterID
		log.Data["cluster_id"] = clusterID
		registrator.ReleaseWaiters()
	}

	err = runAgentMode(ctx, castaiClient, log, cfg, clusterIDHandler)
	if err != nil {
		// it is necessary to log error because invoking of logrus exit handlers will terminate the process using os.Exit()
		// error handling in cobra ("github.com/spf13/cobra") won't be able to log this error
		remoteLogger.Error(err)
	}
	defer castailog.InvokeLogrusExitHandlers(err)
	return err
}

func runAgentMode(ctx context.Context, castaiclient castai.Client, log *logrus.Entry, cfg config.Config, clusterIDChanged func(clusterID string)) error {
	ctx, ctxCancel := context.WithCancel(ctx)
	defer ctxCancel()

	// buffer will allow for all senders to push, even though we will only read first error and cancel context after it;
	// all errors from exitCh are logged
	exitCh := make(chan error, exitChannelBufferSize)
	go watchExitErrors(ctx, log, exitCh, ctxCancel)

	agentVersion := config.VersionInfo
	log.Infof("running agent version: %v", agentVersion)
	log.Infof("platform URL: %s", cfg.API.URL)

	if cfg.PprofPort != 0 {
		closePprof := runPProf(cfg, log, exitCh)
		defer closePprof()
	}

	ctrlHealthz := health.NewHealthzProvider(cfg, log)

	// if pod is holding invalid leader lease, this health check will ensure to kill it by failing pod health check
	leaderWatchDog := leaderelection.NewLeaderHealthzAdaptor(leaderHealthzTimeout)
	checks := map[string]healthz.Checker{
		"ping": healthz.Ping,
	}
	if cfg.LeaderElection.Enabled {
		checks["leader"] = leaderWatchDog.Check
	}
	readinessChecks := lo.Assign(checks, map[string]healthz.Checker{
		"readiness": ctrlHealthz.CheckReadiness,
	})
	livenessChecks := lo.Assign(checks, map[string]healthz.Checker{
		"liveness": ctrlHealthz.CheckLiveness,
	})

	closeHealthz := runHealthzEndpoints(cfg, log, livenessChecks, readinessChecks, exitCh)
	defer closeHealthz()

	closeMetrics := runMetricsEndpoints(cfg, log, exitCh)
	defer closeMetrics()

	restconfig, err := cfg.RetrieveKubeConfig(log)
	if err != nil {
		return err
	}
	if restconfig == nil {
		return fmt.Errorf("kubeconfig is nil")
	}

	if err := v1beta1.AddToScheme(scheme.Scheme); err != nil {
		return fmt.Errorf("adding metrics objs to scheme: %w", err)
	}

	restconfig.NegotiatedSerializer = serializer.NewCodecFactory(scheme.Scheme)

	clientset, err := kubernetes.NewForConfig(restconfig)
	if err != nil {
		return err
	}

	metricsClient, err := versioned.NewForConfig(restconfig)
	if err != nil {
		return fmt.Errorf("initializing metrics client: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(restconfig)
	if err != nil {
		return fmt.Errorf("initializing dynamic client: %w", err)
	}

	discoveryService := discovery.New(clientset, dynamicClient)

	provider, err := providers.GetProvider(ctx, log, discoveryService, dynamicClient)
	if err != nil {
		return fmt.Errorf("getting provider: %w", err)
	}

	metadataStore := metadata.New(clientset, cfg)

	clusterIDChangedHandler := func(clusterID string) {
		if err := metadataStore.StoreMetadataConfigMap(ctx, &metadata.Metadata{
			ClusterID: clusterID,
		}); err != nil {
			log.Warnf("failed to store metadata in a config map: %v", err)
		}

		clusterIDChanged(clusterID)
	}

	log.Data["provider"] = provider.Name()
	log.Infof("using provider %q", provider.Name())

	clusterID := ""
	if cfg.Static != nil {
		clusterID = cfg.Static.ClusterID
	}

	if clusterID != "" {
		log.WithField("cluster_id", clusterID).Info("cluster ID provided by env variable")
	}

	if clusterID == "" {
		reg, err := provider.RegisterCluster(ctx, castaiclient)
		if err != nil {
			return fmt.Errorf("registering cluster: %w", err)
		}
		clusterID = reg.ClusterID
		log.WithField("cluster_id", clusterID).Infof("cluster registered: %v", reg)
	}

	// Final check to ensure we don't run without a cluster ID.
	if clusterID == "" {
		return fmt.Errorf("cluster ID is still empty after initialization")
	}

	clusterIDChangedHandler(clusterID)

	if err := saveMetadata(clusterID, cfg); err != nil {
		return err
	}

	if cfg.LeaderElection.Enabled {
		// Buffered channel to avoid blocking leader election on slow consumers
		leaderStatusCh := make(chan bool, 10)

		params := &controller.Params{
			Log:             log,
			Clientset:       clientset,
			MetricsClient:   metricsClient,
			DynamicClient:   dynamicClient,
			CastaiClient:    castaiclient,
			Provider:        provider,
			ClusterID:       clusterID,
			Config:          cfg,
			AgentVersion:    agentVersion,
			HealthzProvider: ctrlHealthz,
			LeaderStatusCh:  leaderStatusCh,
		}

		go func() {
			if err := controller.RunControllerWithRestart(ctx, params); err != nil {
				exitCh <- fmt.Errorf("controller with restart error: %w", err)
			}
		}()

		// Run leader election with main context
		// If leader election returns an error, we shut down the entire app
		go func() {
			if err := replicas.RunLeaderElection(ctx, log, cfg.LeaderElection, clientset, leaderWatchDog, leaderStatusCh); err != nil {
				exitCh <- fmt.Errorf("leader election error: %w", err)
			}
		}()

		<-ctx.Done()
		log.Info("shutdown signal received, initiating graceful shutdown")
	} else {
		params := &controller.Params{
			Log:             log,
			Clientset:       clientset,
			MetricsClient:   metricsClient,
			DynamicClient:   dynamicClient,
			CastaiClient:    castaiclient,
			Provider:        provider,
			ClusterID:       clusterID,
			Config:          cfg,
			AgentVersion:    agentVersion,
			HealthzProvider: ctrlHealthz,
			LeaderStatusCh:  nil,
		}

		go func() {
			if err := controller.RunControllerWithRestart(ctx, params); err != nil {
				exitCh <- fmt.Errorf("controller with restart error: %w", err)
			}
		}()

		<-ctx.Done()
		log.Info("shutdown signal received, initiating graceful shutdown")
	}

	return nil
}

// if any errors are observed on exitCh, context cancel is called, and all errors in the channel are logged
func watchExitErrors(ctx context.Context, log *logrus.Entry, exitCh chan error, ctxCancel func()) {
	for {
		select {
		case err := <-exitCh:
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Errorf("agent stopped with an error: %v", err)
			}
			ctxCancel()
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				log.Info("shutdown initiated: received termination signal")
			} else {
				log.Infof("context done: %v", ctx.Err())
			}
			return
		}
	}
}

func runPProf(cfg config.Config, log *logrus.Entry, exitCh chan error) (closeFunc func()) {
	log.Infof("starting pprof server on port: %d", cfg.PprofPort)
	addr := portToServerAddr(cfg.PprofPort)
	pprofSrv := &http.Server{Addr: addr, Handler: http.DefaultServeMux}
	closeFn := func() {
		log.Infof("closing pprof server")
		if err := pprofSrv.Close(); err != nil {
			log.Errorf("closing pprof server: %v", err)
		}
	}

	go func() {
		log.Infof("pprof server ready")
		err := pprofSrv.ListenAndServe()
		if err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				exitCh <- fmt.Errorf("pprof server: %w", err)
			} else {
				log.Warnf("pprof server closed")
			}
		}
	}()
	return closeFn
}

func runHealthzEndpoints(
	cfg config.Config,
	log *logrus.Entry,
	livenessChecks map[string]healthz.Checker,
	readinessChecks map[string]healthz.Checker,
	exitCh chan error,
) func() {
	log.Infof("starting healthz server on port: %d", cfg.HealthzPort)

	mux := http.NewServeMux()

	// Handle /healthz (liveness) endpoint
	mux.HandleFunc("/healthz", health.HealthCheckHandler(livenessChecks))

	// Handle /readyz (readiness) endpoint
	mux.HandleFunc("/readyz", health.HealthCheckHandler(readinessChecks))

	addr := portToServerAddr(cfg.HealthzPort)
	healthzSrv := &http.Server{Addr: addr, Handler: mux}
	closeFunc := func() {
		log.Infof("closing healthz server")
		if err := healthzSrv.Close(); err != nil {
			log.Errorf("closing healthz server: %v", err)
		}
	}

	go func() {
		log.Infof("healthz server ready")
		err := healthzSrv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			exitCh <- fmt.Errorf("healthz server: %w", err)
		} else {
			log.Warnf("healthz server closed")
		}
	}()
	return closeFunc
}

func runMetricsEndpoints(cfg config.Config, log *logrus.Entry, exitCh chan error) func() {
	log.Infof("starting metrics server on port: %d", cfg.MetricsPort)
	addr := portToServerAddr(cfg.MetricsPort)

	metricsMux := http.NewServeMux()

	metricsMux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		gatherer := prometheus.Gatherers{
			legacyregistry.DefaultGatherer,
			metrics.Registry,
		}

		promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{
			ErrorLog: log,
		}).ServeHTTP(w, r)
	})

	metricsSrv := &http.Server{Addr: addr, Handler: metricsMux}
	closeFunc := func() {
		log.Infof("closing metrics server")
		if err := metricsSrv.Close(); err != nil {
			log.Errorf("closing metrics server: %v", err)
		}
	}

	go func() {
		log.Infof("metrics server ready")
		err := metricsSrv.ListenAndServe()

		if err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				exitCh <- fmt.Errorf("metrics server: %w", err)
			} else {
				log.Warnf("metrics server closed")
			}
		}
	}()
	return closeFunc
}

func portToServerAddr(port int) string {
	return fmt.Sprintf(":%d", port)
}

func saveMetadata(clusterID string, cfg config.Config) error {
	metadataObj := monitor.Metadata{
		ClusterID: clusterID,
		ProcessID: uint64(os.Getpid()),
	}
	if err := metadataObj.Save(cfg.MonitorMetadata); err != nil {
		return fmt.Errorf("saving metadata: %w", err)
	}
	return nil
}
