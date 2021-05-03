package main

import (
	"context"
	"fmt"
	"io/ioutil"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/collector"
	"castai-agent/internal/services/providers"
	"castai-agent/internal/services/worker"
)

func main() {
	log := logrus.New()
	log.Info("starting the agent")

	if err := run(signals.SetupSignalHandler(), log); err != nil {
		log.Fatalf("agent failed: %v", err)
	}
}

func run(ctx context.Context, log logrus.FieldLogger) error {
	provider, err := providers.GetProvider(ctx, log)
	if err != nil {
		return fmt.Errorf("getting provider: %w", err)
	}

	log = log.WithField("provider", provider.Name())

	castclient := castai.NewClient(log, castai.NewDefaultClient())

	reg, err := provider.RegisterCluster(ctx, castclient)
	if err != nil {
		return fmt.Errorf("registering cluster: %w", err)
	}

	restconfig, err := retrieveKubeConfig()
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(restconfig)
	if err != nil {
		return err
	}

	col, err := collector.NewCollector(log, clientset)
	if err != nil {
		return fmt.Errorf("initializing snapshot collector: %w", err)
	}

	return worker.Run(ctx, log, reg, col, castclient, provider)
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
