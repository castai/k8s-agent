package aks

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"

	"castai-agent/internal/castai"
	"castai-agent/internal/services/providers/types"
	"github.com/sirupsen/logrus"
)

type Provider struct {
	log logrus.FieldLogger
}

const Name = "aks"

func New(log logrus.FieldLogger) (types.Provider, error) {
	return &Provider{
		log: log,
	}, nil
}

func (p *Provider) RegisterCluster(ctx context.Context, client castai.Client) (*types.ClusterRegistration, error) {
	resp, err := client.RegisterCluster(ctx, &castai.RegisterClusterRequest{
		Name: "matas-test-hello",
		AKS: &castai.AKSParams{
			ClusterName: "matas-test",
			Region: "",
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
	return false, nil
}

func (p *Provider) Name() string {
	return Name
}
