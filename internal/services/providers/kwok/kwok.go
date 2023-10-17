package kwok

import (
	"castai-agent/internal/castai"
	"castai-agent/internal/services/providers/types"
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
)

const (
	Name = "kwok"
)

// New configures and returns an kwok provider.
func New(log logrus.FieldLogger) (types.Provider, error) {
	return &Provider{
		log: log,
	}, nil
}

// Provider is the KWOK implementation of the providers.Provider interface.
type Provider struct {
	log logrus.FieldLogger
}

func (p *Provider) RegisterCluster(ctx context.Context, client castai.Client) (*types.ClusterRegistration, error) {
	resp, err := client.RegisterCluster(ctx, &castai.RegisterClusterRequest{
		Name: Name,
		KWOK: &castai.KWOKParams{},
	})
	if err != nil {
		return nil, fmt.Errorf("requesting castai api: %w", err)
	}

	return &types.ClusterRegistration{
		ClusterID:      resp.ID,
		OrganizationID: resp.OrganizationID,
	}, nil
}

func (p *Provider) FilterSpot(ctx context.Context, nodes []*v1.Node) ([]*v1.Node, error) {
	return nodes, nil
}

func (p *Provider) Name() string {
	return Name
}
