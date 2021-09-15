package castai

import (
	"context"
	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/providers/types"
	"castai-agent/pkg/labels"
)

func New(_ context.Context, log logrus.FieldLogger) (types.Provider, error) {
	return &Provider{log: log}, nil
}

type Provider struct {
	log logrus.FieldLogger
}

func (p *Provider) IsSpot(_ context.Context, node *v1.Node) (bool, error) {
	if val, ok := node.Labels[labels.CastaiSpot]; ok && val == "true" {
		return true, nil
	}
	return false, nil
}

func (p *Provider) RegisterCluster(_ context.Context, _ castai.Client) (*types.ClusterRegistration, error) {
	cfg := config.Get().CASTAI
	return &types.ClusterRegistration{
		ClusterID:      cfg.ClusterID,
		OrganizationID: cfg.OrganizationID,
	}, nil
}

func (p *Provider) Name() string {
	return config.ProviderCASTAI
}
