package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/collector"
	"castai-agent/internal/services/providers"
	"castai-agent/internal/services/providers/types"
	"castai-agent/pkg/labels"
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

	const interval = 15 * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := collect(ctx, log, reg, col, provider, castclient); err != nil {
			log.Errorf("collecting snapshot data: %v", err)
		}

		select {
		case <-ticker.C:
		case <-ctx.Done():
			log.Info("shutting down agent")
			return nil
		}
	}
}

func collect(
	ctx context.Context,
	log logrus.FieldLogger,
	reg *types.ClusterRegistration,
	col collector.Collector,
	provider types.Provider,
	castclient castai.Client,
) error {
	cd, err := col.Collect(ctx)
	if err != nil {
		return err
	}

	accountID, err := provider.AccountID(ctx)
	if err != nil {
		return fmt.Errorf("getting account id: %w", err)
	}

	clusterName, err := provider.ClusterName(ctx)
	if err != nil {
		return fmt.Errorf("getting cluster name: %w", err)
	}

	region, err := provider.ClusterRegion(ctx)
	if err != nil {
		return fmt.Errorf("getting cluster region: %w", err)
	}

	snap := &castai.Snapshot{
		ClusterID:       reg.ClusterID,
		OrganizationID:  reg.OrganizationID,
		ClusterProvider: strings.ToUpper(provider.Name()),
		AccountID:       accountID,
		ClusterName:     clusterName,
		ClusterRegion:   region,
		ClusterData:     cd,
	}

	if v := col.GetVersion(); v != nil {
		snap.ClusterVersion = v.Major + "." + v.Minor
	}

	if err := addSpotLabel(ctx, provider, snap.NodeList); err != nil {
		log.Errorf("adding spot labels: %v", err)
	}

	if err := castclient.SendClusterSnapshot(ctx, snap); err != nil {
		return fmt.Errorf("sending cluster snapshot: %w", err)
	}

	return nil
}

func addSpotLabel(ctx context.Context, provider types.Provider, nodes *v1.NodeList) error {
	nodeMap := make(map[string]*v1.Node, len(nodes.Items))
	items := make([]*v1.Node, len(nodes.Items))
	for i, node := range nodes.Items {
		items[i] = &nodes.Items[i]
		nodeMap[node.Name] = &nodes.Items[i]
	}

	spotNodes, err := provider.FilterSpot(ctx, items)
	if err != nil {
		return fmt.Errorf("filtering spot instances: %w", err)
	}

	for _, node := range spotNodes {
		nodeMap[node.Name].Labels[labels.Spot] = "true"
	}

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
