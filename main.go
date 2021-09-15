package main

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"time"

	castailog "castai-agent/pkg/log"

	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/controller"
	"castai-agent/internal/services/providers"
	"castai-agent/internal/services/version"
)

// These should be set via `go build` during a release
var (
	GitCommit = "undefined"
	GitRef    = "no-ref"
	Version   = "local"
)

const LogExporterSendTimeoutSeconds = 15

func main() {
	cfg := config.Get()

	logger := logrus.New()
	logger.SetLevel(logrus.Level(cfg.Log.Level))

	castaiclient := castai.NewClient(logger, castai.NewDefaultClient())

	log := logrus.WithFields(logrus.Fields{})
	if err := run(signals.SetupSignalHandler(), cfg, castaiclient, logger); err != nil {
		logErr := &logContextErr{}
		if errors.As(err, &logErr) {
			log = logger.WithFields(logErr.fields)
		}
		log.Fatalf("agent failed: %v", err)
	}
}

func run(ctx context.Context, cfg *config.Config, castaiclient castai.Client, logger *logrus.Logger) (reterr error) {
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

	agentVersion := &config.AgentVersion{
		GitCommit: GitCommit,
		GitRef:    GitRef,
		Version:   Version,
	}

	fields["version"] = agentVersion.Version
	log := logger.WithFields(fields)
	log.Infof("running agent version: %v", agentVersion)

	restconfig, err := retrieveKubeConfig(log)
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(restconfig)
	if err != nil {
		return err
	}

	provider, err := providers.GetProvider(ctx, cfg, log, clientset)
	if err != nil {
		return fmt.Errorf("getting provider: %w", err)
	}

	fields["provider"] = provider.Name()
	log.Infof("using provider %q", provider.Name())

	reg, err := provider.RegisterCluster(ctx, castaiclient)
	if err != nil {
		return fmt.Errorf("registering cluster: %w", err)
	}

	castailog.SetupLogExporter(logger, castaiclient, castailog.Config{
		ClusterID:          reg.ClusterID,
		MsgSendTimeoutSecs: LogExporterSendTimeoutSeconds,
	})

	fields["cluster_id"] = reg.ClusterID
	log = log.WithFields(fields)
	log.Infof("cluster registered: %v", reg)

	e := echo.New()
	e.HideBanner = true

	wait.Until(func() {
		v, err := version.Get(log, clientset)
		if err != nil {
			panic(fmt.Errorf("failed getting kubernetes version: %v", err))
		}

		fields["k8s_version"] = v.Full()
		log = log.WithFields(fields)

		f := informers.NewSharedInformerFactory(clientset, 0)
		ctrl := controller.New(log, f, castaiclient, provider, reg.ClusterID, 15*time.Second, 30*time.Second, v, agentVersion)
		f.Start(ctx.Done())
		ctrl.Run(ctx)
	}, 0, ctx.Done())

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
