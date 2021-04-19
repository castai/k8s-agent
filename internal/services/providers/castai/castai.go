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

const (
	Name = "castai"
)

func New(_ context.Context, log logrus.FieldLogger) (types.Provider, error) {
	return &Provider{log: log}, nil
}

type Provider struct {
	log logrus.FieldLogger
}

func (p *Provider) RegisterCluster(_ context.Context, _ castai.Client) (*types.ClusterRegistration, error) {
	cfg := config.Get().CASTAI
	return &types.ClusterRegistration{
		ClusterID:      cfg.ClusterID,
		OrganizationID: cfg.OrganizationID,
	}, nil
}

func (p *Provider) Name() string {
	return Name
}

func (p *Provider) FilterSpot(_ context.Context, nodes []*v1.Node) ([]*v1.Node, error) {
	var spots []*v1.Node

	for _, n := range nodes {
		if val, ok := n.ObjectMeta.Labels[labels.Spot]; ok && val == "true" {
			spots = append(spots, n)
		}
	}

	return spots, nil
}

func (p *Provider) AccountID(_ context.Context) (string, error) {
	return "", nil
}

func (p *Provider) ClusterName(_ context.Context) (string, error) {
	return "", nil
}

func (p *Provider) ClusterRegion(_ context.Context) (string, error) {
	return "", nil
}
