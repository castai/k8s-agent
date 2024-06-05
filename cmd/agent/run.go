package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"k8s.io/metrics/pkg/client/clientset/versioned"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/controller"
	"castai-agent/internal/services/controller/scheme"
	"castai-agent/internal/services/discovery"
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

	remoteLogger := logrus.New()
	remoteLogger.SetLevel(logrus.Level(cfg.Log.Level))
	log := remoteLogger.WithField("version", ctx.Value("agentVersion").(*config.AgentVersion).Version)

	localLog := logrus.New()
	localLog.SetLevel(logrus.DebugLevel)

	loggingConfig := castailog.Config{
		SendTimeout: cfg.Log.ExporterSenderTimeout,
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
	// to invoke exit handlers set up in castailog.SetupLogExporter
	defer logrus.Exit(0)

	clusterIDHandler := func(clusterID string) {
		loggingConfig.ClusterID = clusterID
		log.Data["cluster_id"] = clusterID
		registrator.ReleaseWaiters()
	}

	return runAgentMode(ctx, castaiClient, log, cfg, clusterIDHandler)
}

func runAgentMode(ctx context.Context, castaiclient castai.Client, log *logrus.Entry, cfg config.Config, clusterIDChanged func(clusterID string)) error {
	ctx, ctxCancel := context.WithCancel(ctx)
	defer ctxCancel()

	agentVersion := ctx.Value("agentVersion").(*config.AgentVersion)

	// buffer will allow for all senders to push, even though we will only read first error and cancel context after it;
	// all errors from exitCh are logged
	exitCh := make(chan error, 10)
	go watchExitErrors(ctx, log, exitCh, ctxCancel)

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

	log.Data["provider"] = provider.Name()
	log.Infof("using provider %q", provider.Name())

	agentLoopFunc := func(ctx context.Context) error {
		clusterID := ""
		if cfg.Static != nil {
			clusterID = cfg.Static.ClusterID
		}

		if clusterID == "" {
			reg, err := provider.RegisterCluster(ctx, castaiclient)
			if err != nil {
				return fmt.Errorf("registering cluster: %w", err)
			}
			clusterID = reg.ClusterID
			clusterIDChanged(clusterID)
			log.Infof("cluster registered: %v, clusterID: %s", reg, clusterID)
		} else {
			clusterIDChanged(clusterID)
			log.Infof("clusterID: %s provided by env variable", clusterID)
		}

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
	addr := portToServerAddr(cfg.PprofPort)
	pprofSrv := &http.Server{Addr: addr, Handler: http.DefaultServeMux}
	closeFn := func() {
		if err := pprofSrv.Close(); err != nil {
			log.Errorf("closing pprof server: %v", err)
		}
	}

	go func() {
		log.Infof("starting pprof server on %s", addr)
		exitCh <- fmt.Errorf("pprof server: %w", pprofSrv.ListenAndServe())
	}()
	return closeFn
}

func runHealthzEndpoints(cfg config.Config, log *logrus.Entry, checks map[string]healthz.Checker, exitCh chan error) func() {
	log.Infof("starting healthz on port: %d", cfg.HealthzPort)
	allChecks := lo.Assign(map[string]healthz.Checker{
		"server": healthz.Ping,
	}, checks)

	healthzSrv := &http.Server{Addr: portToServerAddr(cfg.HealthzPort), Handler: &healthz.Handler{Checks: allChecks}}
	closeFunc := func() {
		if err := healthzSrv.Close(); err != nil {
			log.Errorf("closing healthz server: %v", err)
		}
	}

	go func() {
		exitCh <- fmt.Errorf("healthz server: %w", healthzSrv.ListenAndServe())
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
