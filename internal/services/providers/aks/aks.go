package aks

import (
	"castai-agent/internal/config"
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"

	"castai-agent/internal/castai"
	"castai-agent/internal/services/providers/aks/metadata"
	"castai-agent/internal/services/providers/types"
	"github.com/sirupsen/logrus"
)

type Provider struct {
	log logrus.FieldLogger
	client metadata.Client
}

const (
	Name = "aks"
	SpotLabelKey = "kubernetes.azure.com/scalesetpriority"
	SpotLabelVal = "spot"
)

func New(log logrus.FieldLogger) (types.Provider, error) {
	return &Provider{
		log: log,
	}, nil
}

func (p *Provider) RegisterCluster(ctx context.Context, client castai.Client) (*types.ClusterRegistration, error) {
	cfg, err := p.clusterAutodiscovery(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := client.RegisterCluster(ctx, &castai.RegisterClusterRequest{
		AKS: &castai.AKSParams{
			Region: cfg.Location,
			ResourceGroup: cfg.ResourceGroup,
			SubscriptionID: cfg.SubscriptionID,
		},
	})

	if err != nil {
		return nil, fmt.Errorf("requesting castai api: %w", err)
	}

	return &types.ClusterRegistration{
		ClusterID:      resp.ID,
		OrganizationID: resp.OrganizationID,
	}, nil
}

func (p *Provider) clusterAutodiscovery(ctx context.Context) (*config.AKS, error){
	var err error
	cfg := &config.AKS{}

	if envCfg := config.Get().AKS; envCfg != nil {
		cfg = envCfg
		if cfg.Location == "" {
			cfg.Location, err = p.client.GetLocation(ctx)
			if err != nil {
				return nil, failedAutodiscovery(err, "AKS_LOCATION")
			}
		}
		if cfg.ResourceGroup == "" {
			cfg.ResourceGroup, err = p.client.GetResourceGroup(ctx)
			if err != nil {
				return nil, failedAutodiscovery(err, "AKS_RESOURCE_GROUP")
			}
		}
		if cfg.SubscriptionID == "" {
			cfg.SubscriptionID, err = p.client.GetSubscriptionID(ctx)
			if err != nil {
				return nil, failedAutodiscovery(err, "AKS_SUBSCRIPTION_ID")
			}
		}
	}

	return cfg, nil
}

func failedAutodiscovery(err error, envVar string) error {
	return fmt.Errorf("autodiscovering cluster metadata: %w\nProvide required %s environment variable", err, envVar)
}

func (p *Provider) IsSpot(_ context.Context, node *corev1.Node) (bool, error) {
	if val, ok :=  node.ObjectMeta.Labels["kubernetes.azure.com/scalesetpriority"]; ok {
		return val == "spot", nil
	}
	return false, nil
}

func (p *Provider) Name() string {
	return Name
}
