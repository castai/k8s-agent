package providers

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/dynamic"

	"castai-agent/internal/config"
	"castai-agent/internal/services/discovery"
	"castai-agent/internal/services/providers/aks"
	"castai-agent/internal/services/providers/eks"
	"castai-agent/internal/services/providers/gke"
	"castai-agent/internal/services/providers/kops"
	"castai-agent/internal/services/providers/openshift"
	"castai-agent/internal/services/providers/types"
)

func GetProvider(ctx context.Context, log logrus.FieldLogger, discoveryService discovery.Service, dyno dynamic.Interface) (types.Provider, error) {
	cfg := config.Get()

	if cfg.Provider == eks.Name || cfg.EKS != nil {
		eksProviderLogger := log.WithField("provider", eks.Name)
		apiNodeLifecycleDiscoveryEnabled := *cfg.EKS.APINodeLifecycleDiscoveryEnabled
		if !apiNodeLifecycleDiscoveryEnabled {
			eksProviderLogger.Info("node lifecycle discovery through AWS API is disabled - all nodes without spot labels will be considered on-demand")
		}

		return eks.New(ctx, eksProviderLogger, apiNodeLifecycleDiscoveryEnabled)
	}

	if cfg.Provider == gke.Name || cfg.GKE != nil {
		return gke.New(log.WithField("provider", gke.Name))
	}

	if cfg.Provider == kops.Name || cfg.KOPS != nil {
		return kops.New(log.WithField("provider", kops.Name), discoveryService)
	}

	if cfg.Provider == aks.Name || cfg.AKS != nil {
		return aks.New(log.WithField("provider", aks.Name))
	}

	if cfg.Provider == openshift.Name {
		return openshift.New(discoveryService, dyno), nil
	}

	return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
}
