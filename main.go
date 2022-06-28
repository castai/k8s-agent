package main

import (
	"castai-agent/internal/services/replicas"
	castailog "castai-agent/pkg/log"
	"context"
	"fmt"
	"io/ioutil"
	"k8s.io/client-go/tools/leaderelection"
	"net/http"
	_ "net/http/pprof"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/controller"
	"castai-agent/internal/services/providers"
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
	log := logrus.WithField("version", Version)

	localLog := logrus.New()
	localLog.SetLevel(logrus.DebugLevel)

	loggingConfig := castailog.Config{
		SendTimeout: LogExporterSendTimeout,
	}

	castaiClient := castai.NewClient(log, castai.NewDefaultRestyClient(), castai.NewDefaultDeltaHTTPClient())

	castailog.SetupLogExporter(remoteLogger, localLog, castaiClient, &loggingConfig)

	clusterIDHandler := func(clusterID string) {
		loggingConfig.ClusterID = clusterID
		log.Data["cluster_id"] = clusterID
	}

	if err := run(signals.SetupSignalHandler(), castaiClient, log, cfg, clusterIDHandler); err != nil {
		log.Fatalf("agent failed: %v", err)
	}

	log.Info("agent shutdown")
}

func run(ctx context.Context, castaiclient castai.Client, log *logrus.Entry, cfg config.Config, clusterIDChanged func(clusterID string)) (reterr error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	agentVersion := &config.AgentVersion{
		GitCommit: GitCommit,
		GitRef:    GitRef,
		Version:   Version,
	}

	exitCh := make(chan error)
	defer close(exitCh)

	log.Infof("running agent version: %v", agentVersion)

	if cfg.PprofPort != 0 {
		closePprof := runPProf(cfg, log, exitCh)
		defer closePprof()
	}

	ctrlHealthz := controller.NewHealthzProvider(cfg)

	// if pod is holding invalid leader lease, this health check will ensure to kill it by failing pod health check
	leaderWatchDog := leaderelection.NewLeaderHealthzAdaptor(time.Minute * 2)

	runHealthzEndpoints(log, ctrlHealthz.Check, leaderWatchDog.Check, exitCh)

	restconfig, err := retrieveKubeConfig(log)
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(restconfig)
	if err != nil {
		return err
	}

	provider, err := providers.GetProvider(ctx, log, clientset)
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

		w := &controller.Worker{
			Fn: controller.RunController(log, clientset, castaiclient, provider, clusterID, cfg, agentVersion, ctrlHealthz),
		}
		defer w.Stop(log)

		if err := w.Start(ctx); err != nil {
			return fmt.Errorf("controller loop error: %w", err)
		}

		return nil
	}

	go func() {
		replicas.Run(ctx, log, config.LeaderElectionConfig{
			LockName:  "agent-leader-election-lock",
			Namespace: "castai-agent",
		}, clientset, leaderWatchDog, func(ctx context.Context) {
			exitCh <- leaderFunc(ctx)
		})

		exitCh <- nil
	}()

	select {
	case err := <-exitCh:
		return err
	case <-ctx.Done():
		return nil
	}
}

func runPProf(cfg config.Config, log *logrus.Entry, exitCh chan error) (closeFunc func()) {
	addr := fmt.Sprintf(":%d", cfg.PprofPort)
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

func runHealthzEndpoints(log *logrus.Entry, controllerCheck healthz.Checker, leaderCheck healthz.Checker, exitCh chan error) {
	healthzSrv := &http.Server{Addr: ":9876", Handler: &healthz.Handler{Checks: map[string]healthz.Checker{
		"server":     healthz.Ping,
		"controller": controllerCheck,
		"leader":     leaderCheck,
	}}}
	defer func() {
		if err := healthzSrv.Close(); err != nil {
			log.Errorf("closing healthz server: %v", err)
		}
	}()

	go func() {
		exitCh <- fmt.Errorf("healthz server: %w", healthzSrv.ListenAndServe())
	}()
}

func kubeConfigFromEnv() (*rest.Config, error) {
	kubepath := config.Get().Kubeconfig
	if kubepath == "" {
		return nil, nil
	}

	data, err := ioutil.ReadFile(kubepath)
	if err != nil {
		return nil, fmt.Errorf("reading kubeconfig at %s: %w", kubepath, err)
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(data)
	if err != nil {
		return nil, fmt.Errorf("building rest config from kubeconfig at %s: %w", kubepath, err)
	}

	return restConfig, nil
}

func retrieveKubeConfig(log logrus.FieldLogger) (*rest.Config, error) {
	kubeconfig, err := kubeConfigFromEnv()
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
