package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptrace"
	_ "net/http/pprof"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/controller"
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

	// Create trace struct.
	trace, debug := trace()
	ctx := httptrace.WithClientTrace(signals.SetupSignalHandler(), trace)

	if err := run(ctx, castaiClient, log, cfg, clusterIDHandler); err != nil {
		// Print debug report.
		debugData, err := json.MarshalIndent(debug, "", "    ")
		log.Print(string(debugData))

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

	ctrlHealthz := controller.NewHealthzProvider(cfg)

	// if pod is holding invalid leader lease, this health check will ensure to kill it by failing pod health check
	leaderWatchDog := leaderelection.NewLeaderHealthzAdaptor(time.Minute * 2)

	closeHealthz := runHealthzEndpoints(cfg, log, ctrlHealthz.Check, leaderWatchDog.Check, exitCh)
	defer closeHealthz()

	restconfig, err := retrieveKubeConfig(log, cfg)
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

		err = controller.Loop(ctx, log, clientset, castaiclient, provider, clusterID, cfg, agentVersion, ctrlHealthz)
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

type Debug struct {
	DNS struct {
		Start   string       `json:"start"`
		End     string       `json:"end"`
		Host    string       `json:"host"`
		Address []net.IPAddr `json:"address"`
		Error   error        `json:"error"`
	} `json:"dns"`
	Dial struct {
		Start string `json:"start"`
		End   string `json:"end"`
	} `json:"dial"`
	Connection struct {
		Time string `json:"time"`
	} `json:"connection"`
	WroteAllRequestHeaders struct {
		Time string `json:"time"`
	} `json:"wrote_all_request_header"`
	WroteAllRequest struct {
		Time string `json:"time"`
	} `json:"wrote_all_request"`
	FirstReceivedResponseByte struct {
		Time string `json:"time"`
	} `json:"first_received_response_byte"`
}

func trace() (*httptrace.ClientTrace, *Debug) {
	d := &Debug{}

	t := &httptrace.ClientTrace{
		DNSStart: func(info httptrace.DNSStartInfo) {
			d.DNS.Start = time.Now().UTC().String()
			d.DNS.Host = info.Host
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			d.DNS.End = time.Now().UTC().String()
			d.DNS.Address = info.Addrs
			d.DNS.Error = info.Err
		},
		ConnectStart: func(network, addr string) {
			d.Dial.Start = time.Now().UTC().String()
		},
		ConnectDone: func(network, addr string, err error) {
			d.Dial.End = time.Now().UTC().String()
		},
		GotConn: func(connInfo httptrace.GotConnInfo) {
			d.Connection.Time = time.Now().UTC().String()
		},
		WroteHeaders: func() {
			d.WroteAllRequestHeaders.Time = time.Now().UTC().String()
		},
		WroteRequest: func(wr httptrace.WroteRequestInfo) {
			d.WroteAllRequest.Time = time.Now().UTC().String()
		},
		GotFirstResponseByte: func() {
			d.FirstReceivedResponseByte.Time = time.Now().UTC().String()
		},
	}

	return t, d
}
