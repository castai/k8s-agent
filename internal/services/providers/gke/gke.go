package gke

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/providers/gke/client"
	"castai-agent/internal/services/providers/types"
	"castai-agent/pkg/labels"
)

const (
	Name             = "gke"
	LabelPreemptible = "cloud.google.com/gke-preemptible"
	LabelSpot        = "cloud.google.com/gke-spot"
)

func New(log logrus.FieldLogger) (types.Provider, error) {
	return &Provider{
		log:      log,
		metadata: client.NewMetadataClient(),
	}, nil
}

type Provider struct {
	log      logrus.FieldLogger
	metadata client.Metadata
}

func (p *Provider) RegisterCluster(ctx context.Context, client castai.Client) (*types.ClusterRegistration, error) {
	cfg, err := p.clusterAutodiscovery()
	if err != nil {
		return nil, err
	}

	resp, err := client.RegisterCluster(ctx, &castai.RegisterClusterRequest{
		Name: cfg.ClusterName,
		GKE: &castai.GKEParams{
			Region:      cfg.Region,
			ProjectID:   cfg.ProjectID,
			ClusterName: cfg.ClusterName,
			Location:    cfg.Location,
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

func (p *Provider) clusterAutodiscovery() (*config.GKE, error) {
	var err error
	cfg := &config.GKE{}
	if envCfg := config.Get().GKE; envCfg != nil {
		cfg = envCfg
	}

	if cfg.ProjectID == "" {
		cfg.ProjectID, err = p.metadata.GetProjectID()
		if err != nil {
			return nil, failedAutodiscovery(err, "GKE_PROJECT_ID")
		}
	}

	if cfg.ClusterName == "" {
		cfg.ClusterName, err = p.metadata.GetClusterName()
		if err != nil {
			return nil, failedAutodiscovery(err, "GKE_CLUSTER_NAME")
		}
	}

	if cfg.Region == "" {
		cfg.Region, err = p.metadata.GetRegion()
		if err != nil {
			return nil, failedAutodiscovery(err, "GKE_REGION")
		}
	}

	if cfg.Location == "" {
		cfg.Location, err = p.metadata.GetLocation()
		if err != nil {
			return nil, failedAutodiscovery(err, "GKE_LOCATION")
		}
	}

	return cfg, nil
}

func failedAutodiscovery(err error, envVar string) error {
	return fmt.Errorf("autodiscovering cluster metadata: %w\nProvide required %s environment variable", err, envVar)
}

func isSpot(node *corev1.Node) bool {
	if val, ok := node.Labels[labels.CastaiSpot]; ok && val == "true" {
		return true
	}

	if val, ok := node.Labels[LabelPreemptible]; ok && val == "true" {
		return true
	}

	if val, ok := node.Labels[LabelSpot]; ok && val == "true" {
		return true
	}

	if val, ok := node.Labels[labels.KarpenterCapacityType]; ok && val == labels.ValueKarpenterCapacityTypeSpot {
		return true
	}

	return false
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

func (p *Provider) Name() string {
	return Name
}
