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

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/controller"
	"castai-agent/internal/services/controller/scheme"
	"castai-agent/internal/services/discovery"
	"castai-agent/internal/services/metadata"
	"castai-agent/internal/services/metrics"
	"castai-agent/internal/services/monitor"
	"castai-agent/internal/services/providers"
	"castai-agent/internal/services/replicas"
	castailog "castai-agent/pkg/log"
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
			timer := time.NewTicker(*cfg.Log.PrintMemoryUsageEvery)
			for {
				select {
				case <-timer.C:
					var m runtime.MemStats
					runtime.ReadMemStats(&m)
					log.WithFields(logrus.Fields{
						"Alloc_MiB":      m.Alloc / 1024 / 1024,
						"TotalAlloc_MiB": m.TotalAlloc / 1024 / 1024,
						"Sys_MiB":        m.Sys / 1024 / 1024,
						"NumGC":          m.NumGC,
					}).Infof("memory usage")
				case <-ctx.Done():
					break
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
	exitCh := make(chan error, 10)
	go watchExitErrors(ctx, log, exitCh, ctxCancel)

	agentVersion := config.VersionInfo
	log.Infof("running agent version: %v", agentVersion)
	log.Infof("platform URL: %s", cfg.API.URL)

	if cfg.PprofPort != 0 {
		closePprof := runPProf(cfg, log, exitCh)
		defer closePprof()
	}

	ctrlHealthz := controller.NewHealthzProvider(cfg, log)

	// if pod is holding invalid leader lease, this health check will ensure to kill it by failing pod health check
	leaderWatchDog := leaderelection.NewLeaderHealthzAdaptor(time.Minute * 2)
	checks := map[string]healthz.Checker{
		"controller": ctrlHealthz.Check,
	}
	if cfg.LeaderElection.Enabled {
		checks["leader"] = leaderWatchDog.Check
	}

	closeHealthz := runHealthzEndpoints(cfg, log, checks, exitCh)
	defer closeHealthz()

	closeMetrics := runMetricsEndpoints(cfg, log, exitCh)
	defer closeMetrics()

	restconfig, err := cfg.RetrieveKubeConfig(log)
	if err != nil {
		return err
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

	agentLoopFunc := func(ctx context.Context) error {
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
			// This is not normal, but we see some requests with missing cluster IDs. Agent without cluster ID will not be
			// able to upload snapshots which means it's serving no purpose. It's best to raise this issue and get it fixed.
			return fmt.Errorf("cluster ID is still empty after initialization")
		}

		clusterIDChangedHandler(clusterID)

		if err := saveMetadata(clusterID, cfg); err != nil {
			return err
		}

		err = controller.Loop(
			ctx,
			log,
			clientset,
			metricsClient,
			dynamicClient,
			castaiclient,
			provider,
			clusterID,
			cfg,
			agentVersion,
			ctrlHealthz,
		)
		if err != nil {
			return fmt.Errorf("controller loop error: %w", err)
		}

		return nil
	}

	if cfg.LeaderElection.Enabled {
		replicas.Run(ctx, log, cfg.LeaderElection, clientset, leaderWatchDog, func(ctx context.Context) {
			exitCh <- agentLoopFunc(ctx)
		})
	} else {
		exitCh <- agentLoopFunc(ctx)
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
			log.Infof("context done")
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

func runHealthzEndpoints(cfg config.Config, log *logrus.Entry, checks map[string]healthz.Checker, exitCh chan error) func() {
	log.Infof("starting healthz server on port: %d", cfg.HealthzPort)
	allChecks := lo.Assign(map[string]healthz.Checker{
		"server": healthz.Ping,
	}, checks)
	addr := portToServerAddr(cfg.HealthzPort)
	healthzSrv := &http.Server{Addr: addr, Handler: &healthz.Handler{Checks: allChecks}}
	closeFunc := func() {
		log.Infof("closing healthz server")
		if err := healthzSrv.Close(); err != nil {
			log.Errorf("closing healthz server: %v", err)
		}
	}

	go func() {
		log.Infof("healthz server ready")
		err := healthzSrv.ListenAndServe()
		if err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				exitCh <- fmt.Errorf("healthz server: %w", err)
			} else {
				log.Warnf("healthz server closed")
			}
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
	metadata := monitor.Metadata{
		ClusterID: clusterID,
		ProcessID: uint64(os.Getpid()),
	}
	if err := metadata.Save(cfg.MonitorMetadata); err != nil {
		return fmt.Errorf("saving metadata: %w", err)
	}
	return nil
}
