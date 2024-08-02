package aks

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/providers/aks/metadata"
	"castai-agent/internal/services/providers/types"
	"castai-agent/pkg/labels"
)

type Provider struct {
	log    logrus.FieldLogger
	client metadata.Client
}

const (
	Name         = "aks"
	SpotLabelKey = "kubernetes.azure.com/scalesetpriority"
	SpotLabelVal = "spot"
)

func New(log logrus.FieldLogger) (types.Provider, error) {
	return &Provider{
		log:    log,
		client: metadata.NewClient(log),
	}, nil
}

func (p *Provider) RegisterCluster(ctx context.Context, client castai.Client) (*types.ClusterRegistration, error) {
	cfg, err := p.clusterAutodiscovery(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := client.RegisterCluster(ctx, &castai.RegisterClusterRequest{
		AKS: &castai.AKSParams{
			Region:            cfg.Location,
			NodeResourceGroup: cfg.NodeResourceGroup,
			SubscriptionID:    cfg.SubscriptionID,
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

func (p *Provider) clusterAutodiscovery(ctx context.Context) (*config.AKS, error) {
	var err error
	cfg := config.Get().AKS
	if cfg == nil {
		cfg = &config.AKS{}
	}

	if cfg.Location == "" {
		cfg.Location, err = p.client.GetLocation()
		if err != nil {
			return nil, failedAutodiscovery(err, "AKS_LOCATION")
		}
	}
	if cfg.NodeResourceGroup == "" {
		cfg.NodeResourceGroup, err = p.client.GetResourceGroup()
		if err != nil {
			return nil, failedAutodiscovery(err, "AKS_NODE_RESOURCE_GROUP")
		}
	}
	if cfg.SubscriptionID == "" {
		cfg.SubscriptionID, err = p.client.GetSubscriptionID()
		if err != nil {
			return nil, failedAutodiscovery(err, "AKS_SUBSCRIPTION_ID")
		}
	}

	return cfg, nil
}

func failedAutodiscovery(err error, envVar string) error {
	return fmt.Errorf("autodiscovering cluster metadata: %w: provide required %s environment variable", err, envVar)
}

func (p *Provider) FilterSpot(_ context.Context, nodes []*corev1.Node) ([]*corev1.Node, error) {
	var ret []*corev1.Node

	for _, node := range nodes {
		if isSpot(node) {
			ret = append(ret, node)
		}

	}

	return ret, nil
}

func isSpot(node *corev1.Node) bool {
	if val, ok := node.ObjectMeta.Labels["kubernetes.azure.com/scalesetpriority"]; ok && val == "spot" {
		return true
	}

	if val, ok := node.ObjectMeta.Labels[labels.KarpenterCapacityType]; ok && val == labels.ValueKarpenterCapacityTypeSpot {
		return true
	}

	return false
}

func (p *Provider) Name() string {
	return Name
}
