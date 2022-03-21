package providers

import (
	"context"
	"fmt"

	"castai-agent/internal/services/providers/aks"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"

	"castai-agent/internal/config"
	"castai-agent/internal/services/providers/castai"
	"castai-agent/internal/services/providers/eks"
	"castai-agent/internal/services/providers/gke"
	"castai-agent/internal/services/providers/kops"
	"castai-agent/internal/services/providers/types"
)

func GetProvider(ctx context.Context, log logrus.FieldLogger, clientset kubernetes.Interface) (types.Provider, error) {
	cfg := config.Get()

	if cfg.Provider == castai.Name || cfg.CASTAI != nil {
		return castai.New(ctx, log.WithField("provider", castai.Name))
	}

	if cfg.Provider == eks.Name || cfg.EKS != nil {
		return eks.New(ctx, log.WithField("provider", eks.Name))
	}

	if cfg.Provider == gke.Name || cfg.GKE != nil {
		return gke.New(log.WithField("provider", gke.Name))
	}

	if cfg.Provider == kops.Name || cfg.KOPS != nil {
		return kops.New(log.WithField("provider", kops.Name), clientset)
	}

	if cfg.Provider == aks.Name || cfg.AKS != nil {
		return aks.New(log.WithField("provider", aks.Name))
	}

	return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
}
