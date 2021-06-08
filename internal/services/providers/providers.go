package providers

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"castai-agent/internal/config"
	"castai-agent/internal/services/providers/castai"
	"castai-agent/internal/services/providers/eks"
	"castai-agent/internal/services/providers/gke"
	"castai-agent/internal/services/providers/types"
)

func GetProvider(ctx context.Context, log logrus.FieldLogger) (types.Provider, error) {
	cfg := config.Get()

	if cfg.Provider == castai.Name || cfg.CASTAI != nil {
		return castai.New(ctx, log.WithField("provider", castai.Name))
	}

	if cfg.Provider == eks.Name || cfg.EKS != nil {
		return eks.New(ctx, log.WithField("provider", eks.Name))
	}

	if cfg.Provider == gke.Name || cfg.GKE != nil {
		return gke.New(ctx, log.WithField("provider", gke.Name))
	}

	return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
}
