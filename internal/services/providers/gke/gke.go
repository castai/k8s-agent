package gke

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/providers/types"
	"castai-agent/pkg/labels"
)

const (
	Name = "gke"

	LabelPreemptible = "cloud.google.com/gke-preemptible"
)

func New(log logrus.FieldLogger) (types.Provider, error) {
	return &Provider{log: log}, nil
}

type Provider struct {
	log logrus.FieldLogger
}

func (p *Provider) RegisterCluster(ctx context.Context, client castai.Client) (*types.ClusterRegistration, error) {
	cfg := config.Get().GKE

	resp, err := client.RegisterCluster(ctx, &castai.RegisterClusterRequest{
		Name: cfg.ClusterName,
		GKE: &castai.GKEParams{
			Region:      cfg.Region,
			ProjectID:   cfg.ProjectID,
			ClusterName: cfg.ClusterName,
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

func (p *Provider) IsSpot(_ context.Context, node *corev1.Node) (bool, error) {
	if val, ok := node.Labels[labels.Spot]; ok && val == "true" {
		return true, nil
	}

	if val, ok := node.Labels[LabelPreemptible]; ok && val == "true" {
		return true, nil
	}

	return false, nil
}

func (p *Provider) Name() string {
	return Name
}
