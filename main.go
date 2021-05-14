package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"time"

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
	GitCommit string = "undefined"
	GitRef    string = "no-ref"
	Version   string = "local"
)

func main() {
	log := logrus.New()
	log.Info("starting the agent")

	if err := run(signals.SetupSignalHandler(), log); err != nil {
		log.Fatalf("agent failed: %v", err)
	}
}

func run(ctx context.Context, log logrus.FieldLogger) error {
	agentVersion := &config.AgentVersion{
		GitCommit: GitCommit,
		GitRef:    GitRef,
		Version:   Version,
	}
	log.Infof("Running agentVersion: %+v", agentVersion)
	provider, err := providers.GetProvider(ctx, log)
	if err != nil {
		return fmt.Errorf("getting provider: %w", err)
	}

	log = log.WithField("provider", provider.Name())

	castaiclient := castai.NewClient(log, castai.NewDefaultClient())

	reg, err := provider.RegisterCluster(ctx, castaiclient)
	if err != nil {
		return fmt.Errorf("registering cluster: %w", err)
	}

	log = log.WithField("cluster_id", reg.ClusterID)

	restconfig, err := retrieveKubeConfig()
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(restconfig)
	if err != nil {
		return err
	}

	wait.Until(func() {
		v, err := version.Get(log, clientset)
		if err != nil {
			panic(fmt.Errorf("failed getting kubernetes version: %v", err))
		}

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

func retrieveKubeConfig() (*rest.Config, error) {
	kubeconfig, err := kubeConfigFromEnv()
	if err != nil {
		return nil, err
	}

	if kubeconfig != nil {
		return kubeconfig, nil
	}

	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	return inClusterConfig, nil
}
