package anywhere

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/discovery"
	"castai-agent/internal/services/providers/anywhere/client"
	"castai-agent/internal/services/providers/types"
)

const Name = "anywhere"

var _ types.Provider = (*Provider)(nil)

type Provider struct {
	discoveryService discovery.Service
	client           client.Client
	log              logrus.FieldLogger
}

func New(discoveryService discovery.Service, client client.Client, log logrus.FieldLogger) *Provider {
	return &Provider{
		discoveryService: discoveryService,
		client:           client,
		log:              log,
	}
}

func (p *Provider) RegisterCluster(ctx context.Context, client castai.Client) (*types.ClusterRegistration, error) {
	kubeSystemNamespaceId, err := p.discoveryService.GetKubeSystemNamespaceID(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting kube-system namespace id: %w", err)
	}

	clusterName := ""
	if cfg := config.Get().Anywhere; cfg != nil {
		clusterName = cfg.ClusterName
	}

	if clusterName == "" {
		discoveredClusterName, err := p.client.GetClusterName(ctx)
		if err != nil {
			p.log.Errorf("discovering cluster name: %v", err)
		} else if discoveredClusterName != "" {
			clusterName = discoveredClusterName
		}
	}

	req := &castai.RegisterClusterRequest{
		Name: clusterName,
		Anywhere: &castai.AnywhereParams{
			ClusterName:           clusterName,
			KubeSystemNamespaceID: *kubeSystemNamespaceId,
		},
	}
	resp, err := client.RegisterCluster(ctx, req)
	if err != nil {
		return nil, err
	}

	return &types.ClusterRegistration{
		ClusterID:      resp.ID,
		OrganizationID: resp.OrganizationID,
	}, nil
}

func (p *Provider) Name() string {
	return Name
}

func (p *Provider) FilterSpot(_ context.Context, _ []*v1.Node) ([]*v1.Node, error) {
	return nil, nil
}
