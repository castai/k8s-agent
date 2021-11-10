package main

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"castai-agent/cmd/agent-actions/actions"
	"castai-agent/cmd/agent-actions/config"
	"castai-agent/cmd/agent-actions/telemetry"
	"castai-agent/cmd/agent-actions/version"
)

// These should be set via `go build` during a release.
var (
	GitCommit = "undefined"
	GitRef    = "no-ref"
	Version   = "local"
)

const LogExporterSendTimeoutSeconds = 15

func main() {
	cfg := config.Get()

	logger := logrus.New()
	logLevel := logrus.Level(cfg.Log.Level)
	logger.SetLevel(logrus.Level(cfg.Log.Level))

	telemetryClient := telemetry.NewClient(logger, telemetry.NewDefaultClient(cfg.API.TelemetryURL, cfg.API.Key, logLevel))

	log := logrus.WithFields(logrus.Fields{})
	if err := run(signals.SetupSignalHandler(), telemetryClient, logger, cfg); err != nil {
		logErr := &logContextErr{}
		if errors.As(err, &logErr) {
			log = logger.WithFields(logErr.fields)
		}
		log.Fatalf("agent-actions failed: %v", err)
	}
}

func run(ctx context.Context, telemetryClient telemetry.Client, logger *logrus.Logger, cfg config.Config) (reterr error) {
	fields := logrus.Fields{}

	defer func() {
		if reterr == nil {
			return
		}

		reterr = &logContextErr{
			err:    reterr,
			fields: fields,
		}
	}()

	binVersion := &config.AgentActionsVersion{
		GitCommit: GitCommit,
		GitRef:    GitRef,
		Version:   Version,
	}

	fields["version"] = binVersion.Version
	log := logger.WithFields(fields)
	log.Infof("running agent-actions version: %v", binVersion)

	restconfig, err := retrieveKubeConfig(log)
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(restconfig)
	if err != nil {
		return err
	}

	fields["cluster_id"] = cfg.ClusterID
	log = log.WithFields(fields)

	if cfg.PprofPort != 0 {
		go func() {
			addr := fmt.Sprintf(":%d", cfg.PprofPort)
			log.Infof("starting pprof server on %s", addr)
			if err := http.ListenAndServe(addr, http.DefaultServeMux); err != nil {
				log.Errorf("failed to start pprof http server: %v", err)
			}
		}()
	}

	v, err := version.Get(log, clientset)
	if err != nil {
		panic(fmt.Errorf("failed getting kubernetes version: %v", err))
	}

	fields["k8s_version"] = v.Full()
	log = log.WithFields(fields)

	actionsConfig := actions.Config{
		PollInterval:    5 * time.Second,
		PollTimeout:     1 * time.Minute,
		AckTimeout:      30 * time.Second,
		AckRetriesCount: 3,
		AckRetryWait:    1 * time.Second,
		ClusterID:       cfg.ClusterID,
	}
	svc := actions.NewService(log, actionsConfig, clientset, telemetryClient)
	svc.Run(ctx)

	return nil
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

type logContextErr struct {
	err    error
	fields logrus.Fields
}

func (e *logContextErr) Error() string {
	return e.err.Error()
}

func (e *logContextErr) Unwrap() error {
	return e.err
}
