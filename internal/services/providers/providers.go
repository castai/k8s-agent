package providers

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"

	"castai-agent/internal/config"
	"castai-agent/internal/services/providers/castai"
	"castai-agent/internal/services/providers/eks"
	"castai-agent/internal/services/providers/gke"
	"castai-agent/internal/services/providers/kops"
	"castai-agent/internal/services/providers/types"
)

func GetProvider(ctx context.Context, cfg *config.Config, log logrus.FieldLogger, clientset kubernetes.Interface) (types.Provider, error) {
	if cfg.Provider == config.ProviderCASTAI || cfg.CASTAI != nil {
		if cfg.CASTAI == nil {
			return nil, fmt.Errorf("provider configuration not found")
		}
		return castai.New(ctx, log.WithField("provider", config.ProviderCASTAI))
	}

	if cfg.Provider == eks.Name || cfg.EKS != nil {
		if cfg.EKS == nil {
			return nil, fmt.Errorf("provider configuration not found")
		}
		return eks.New(ctx, log.WithField("provider", eks.Name))
	}

	if cfg.Provider == gke.Name || cfg.GKE != nil {
		if cfg.GKE == nil {
			return nil, fmt.Errorf("provider configuration not found")
		}
		return gke.New(log.WithField("provider", gke.Name))
	}

	if cfg.Provider == kops.Name || cfg.KOPS != nil {
		if cfg.KOPS == nil {
			return nil, fmt.Errorf("provider configuration not found")
		}
		return kops.New(log.WithField("provider", kops.Name), clientset)
	}

	return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
}
