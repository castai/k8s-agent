package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	_ "net/http/pprof"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/healthz"

	castailog "castai-agent/pkg/log"

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

	castaiClient := castai.NewClient(log, castai.NewDefaultRestyClient(), castai.NewDefaultDeltaHTTPClient())
	if err := run(signals.SetupSignalHandler(), castaiClient, remoteLogger, log, cfg); err != nil {
		log.Fatalf("agent failed: %v", err)
	}

	log.Info("agent shutdown")
}

func run(ctx context.Context, castaiclient castai.Client, baseLogger *logrus.Logger, log *logrus.Entry, cfg config.Config) (reterr error) {
	localLog := logrus.New()
	localLog.SetLevel(logrus.DebugLevel)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	agentVersion := &config.AgentVersion{
		GitCommit: GitCommit,
		GitRef:    GitRef,
		Version:   Version,
	}

	log.Infof("running agent version: %v", agentVersion)

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
		log.Infof("cluster registered: %v, clusterID: %s", reg, clusterID)
	} else {
		log.Infof("clusterID: %s provided by env variable", clusterID)
	}
	castailog.SetupLogExporter(baseLogger, localLog, castaiclient, castailog.Config{
		ClusterID:   clusterID,
		SendTimeout: LogExporterSendTimeout,
	})

	log.Data["cluster_id"] = clusterID

	exitCh := make(chan error)

	if cfg.PprofPort != 0 {
		addr := fmt.Sprintf(":%d", cfg.PprofPort)
		pprofSrv := &http.Server{Addr: addr, Handler: http.DefaultServeMux}
		defer func() {
			if err := pprofSrv.Close(); err != nil {
				log.Errorf("closing pprof server: %v", err)
			}
		}()

		go func() {
			log.Infof("starting pprof server on %s", addr)
			exitCh <- fmt.Errorf("pprof server: %w", pprofSrv.ListenAndServe())
		}()
	}

	ctrlHealthz := controller.NewHealthzProvider(cfg)

	healthzSrv := &http.Server{Addr: ":9876", Handler: &healthz.Handler{Checks: map[string]healthz.Checker{
		"server":     healthz.Ping,
		"controller": ctrlHealthz.Check,
	}}}
	defer func() {
		if err := healthzSrv.Close(); err != nil {
			log.Errorf("closing healthz server: %v", err)
		}
	}()

	go func() {
		exitCh <- fmt.Errorf("healthz server: %w", healthzSrv.ListenAndServe())
	}()

	w := &controller.Worker{
		Fn: controller.RunController(log, clientset, castaiclient, provider, clusterID, cfg, agentVersion, ctrlHealthz),
	}
	defer w.Stop(log)

	go func() {
		exitCh <- fmt.Errorf("controller loop: %w", w.Start(ctx))
	}()

	select {
	case err := <-exitCh:
		return err
	case <-ctx.Done():
		return nil
	}
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
