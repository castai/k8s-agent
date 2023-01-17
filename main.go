package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"k8s.io/metrics/pkg/client/clientset/versioned"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/controller"
	"castai-agent/internal/services/discovery"
	"castai-agent/internal/services/providers"
	"castai-agent/internal/services/replicas"
	castailog "castai-agent/pkg/log"
)

// These should be set via `go build` during a release
var (
	GitCommit = "undefined"
	GitRef    = "no-ref"
	Version   = "local"
)

const LogExporterSendTimeout = 15 * time.Second

func main() {
	cfg := config.Get()

	remoteLogger := logrus.New()
	remoteLogger.SetLevel(logrus.Level(cfg.Log.Level))
	log := remoteLogger.WithField("version", Version)

	localLog := logrus.New()
	localLog.SetLevel(logrus.DebugLevel)

	loggingConfig := castailog.Config{
		SendTimeout: LogExporterSendTimeout,
	}

	castaiClient := castai.NewClient(log, castai.NewDefaultRestyClient(), castai.NewDefaultDeltaHTTPClient())

	waitForRegistrationCh := make(chan struct{})
	var registrationChClose sync.Once
	registrationChCloseBody := func() {
		close(waitForRegistrationCh)
	}
	defer func() {
		registrationChClose.Do(registrationChCloseBody)
	}()
	castailog.SetupLogExporter(waitForRegistrationCh, remoteLogger, localLog, castaiClient, &loggingConfig)

	clusterIDHandler := func(clusterID string) {
		loggingConfig.ClusterID = clusterID
		log.Data["cluster_id"] = clusterID
		registrationChClose.Do(registrationChCloseBody)
	}

	ctx := signals.SetupSignalHandler()
	if err := run(ctx, castaiClient, log, cfg, clusterIDHandler); err != nil {
		log.Fatalf("agent failed: %v", err)
	}

	log.Info("agent shutdown")
}

func run(ctx context.Context, castaiclient castai.Client, log *logrus.Entry, cfg config.Config, clusterIDChanged func(clusterID string)) error {
	ctx, ctxCancel := context.WithCancel(ctx)
	defer ctxCancel()

	agentVersion := &config.AgentVersion{
		GitCommit: GitCommit,
		GitRef:    GitRef,
		Version:   Version,
	}

	// buffer will allow for all senders to push, even though we will only read first error and cancel context after it
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

	closeHealthz := runHealthzEndpoints(cfg, log, ctrlHealthz.Check, leaderWatchDog.Check, exitCh)
	defer closeHealthz()

	restconfig, err := retrieveKubeConfig(log, cfg)
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

	leaderFunc := func(ctx context.Context) error {
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

		err = controller.Loop(
			ctx,
			log,
			clientset,
			metricsClient,
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

	replicas.Run(ctx, log, cfg.LeaderElection, clientset, leaderWatchDog, func(ctx context.Context) {
		exitCh <- leaderFunc(ctx)
	})
	return nil
}

// if any errors are observed on exitCh, context cancel is called, and all errors in the channel are logged
func watchExitErrors(ctx context.Context, log *logrus.Entry, exitCh chan error, ctxCancel func()) {
	select {
	case err := <-exitCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Errorf("agent stopped with an error: %v", err)
		}
		ctxCancel()
	case <-ctx.Done():
		return
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

func runHealthzEndpoints(cfg config.Config, log *logrus.Entry, controllerCheck healthz.Checker, leaderCheck healthz.Checker, exitCh chan error) func() {
	log.Infof("starting healthz on port: %d", cfg.HealthzPort)
	healthzSrv := &http.Server{Addr: portToServerAddr(cfg.HealthzPort), Handler: &healthz.Handler{Checks: map[string]healthz.Checker{
		"server":     healthz.Ping,
		"controller": controllerCheck,
		"leader":     leaderCheck,
	}}}
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

func kubeConfigFromPath(kubepath string) (*rest.Config, error) {
	if kubepath == "" {
		return nil, nil
	}

	data, err := os.ReadFile(kubepath)
	if err != nil {
		return nil, fmt.Errorf("reading kubeconfig at %s: %w", kubepath, err)
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(data)
	if err != nil {
		return nil, fmt.Errorf("building rest config from kubeconfig at %s: %w", kubepath, err)
	}

	return restConfig, nil
}

func retrieveKubeConfig(log logrus.FieldLogger, cfg config.Config) (*rest.Config, error) {
	kubeconfig, err := kubeConfigFromPath(cfg.Kubeconfig)
	if err != nil {
		return nil, err
	}

	if kubeconfig != nil {
		log.Debug("using kubeconfig from env variables")
		return kubeconfig, nil
	}

	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	log.Debug("using in cluster kubeconfig")
	return inClusterConfig, nil
}
