package providers

import (
	"context"
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"

	"castai-agent/internal/config"
	"castai-agent/internal/services/providers/castai"
	"castai-agent/internal/services/providers/eks"
	"castai-agent/internal/services/providers/types"
)

func GetProvider(ctx context.Context, log logrus.FieldLogger) (p types.Provider, err error) {
	cfg := config.Get()

	if cfg.Provider == castai.Name || cfg.CASTAI != nil {
		return castai.New(ctx, log)
	}

	if cfg.Provider == eks.Name || cfg.EKS != nil {
		return eks.New(ctx, log)
	}

	if cfg.Provider == "" {
		return dynamicProvider(ctx, log)
	}

	return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
}

func dynamicProvider(ctx context.Context, log logrus.FieldLogger) (types.Provider, error) {
	log.Info("using cluster provider discovery")

	if p, err := eks.New(ctx, log); err != nil {
		log.Info(fmt.Errorf("eks did not succeed in discovery: %v", err))
	} else {
		return p, nil
	}

	return nil, errors.New("none of the providers supporting discovery were able to initialize")
}
