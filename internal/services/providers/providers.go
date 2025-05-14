package providers

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/dynamic"

	"castai-agent/internal/config"
	"castai-agent/internal/services/discovery"
	"castai-agent/internal/services/providers/aks"
	"castai-agent/internal/services/providers/anywhere"
	anywhere_client "castai-agent/internal/services/providers/anywhere/client"
	"castai-agent/internal/services/providers/eks"
	"castai-agent/internal/services/providers/gke"
	"castai-agent/internal/services/providers/kops"
	"castai-agent/internal/services/providers/openshift"
	"castai-agent/internal/services/providers/selfhostedec2"
	"castai-agent/internal/services/providers/types"
)

func GetProvider(ctx context.Context, log logrus.FieldLogger, discoveryService discovery.Service, dyno dynamic.Interface) (types.Provider, error) {
	cfg := config.Get()

	if cfg.Provider == eks.Name || cfg.EKS != nil {
		eksProviderLogger := log.WithField("provider", eks.Name)
		apiNodeLifecycleDiscoveryEnabled := isAPINodeLifecycleDiscoveryEnabled(cfg)

		if !apiNodeLifecycleDiscoveryEnabled {
			eksProviderLogger.Info("node lifecycle discovery through AWS API is disabled - all nodes without spot labels will be considered on-demand")
		}

		return eks.New(ctx, eksProviderLogger, apiNodeLifecycleDiscoveryEnabled)
	}

	if cfg.Provider == selfhostedec2.Name || cfg.SelfHostedEC2 != nil {
		providerLogger := log.WithField("provider", selfhostedec2.Name)

		apiNodeLifecycleDiscoveryEnabled := config.DefaultAPINodeLifecycleDiscoveryEnabled
		if cfg.SelfHostedEC2 != nil && cfg.SelfHostedEC2.APINodeLifecycleDiscoveryEnabled != nil {
			apiNodeLifecycleDiscoveryEnabled = *cfg.SelfHostedEC2.APINodeLifecycleDiscoveryEnabled
		}

		if !apiNodeLifecycleDiscoveryEnabled {
			providerLogger.Info("node lifecycle discovery through AWS API is disabled - all nodes without spot labels will be considered on-demand")
		}

		return selfhostedec2.New(ctx, providerLogger, apiNodeLifecycleDiscoveryEnabled)
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

	if cfg.Provider == anywhere.Name || cfg.Anywhere != nil {
		logger := log.WithField("provider", cfg.Provider)
		client := anywhere_client.New(log, discoveryService)

		return anywhere.New(discoveryService, client, logger), nil
	}

	if cfg.Provider == openshift.Name {
		return openshift.New(discoveryService, dyno), nil
	}

	return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
}

func isAPINodeLifecycleDiscoveryEnabled(cfg config.Config) bool {
	if cfg.EKS != nil && cfg.EKS.APINodeLifecycleDiscoveryEnabled != nil {
		return *cfg.EKS.APINodeLifecycleDiscoveryEnabled
	}

	return config.DefaultAPINodeLifecycleDiscoveryEnabled
}
