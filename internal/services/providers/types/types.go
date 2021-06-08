//go:generate mockgen -destination ./mock/provider.go . Provider
package types

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"

	castclient "castai-agent/internal/castai"
)

// Provider is an abstraction for various CAST AI supported K8s providers, like EKS, GKE, etc.
type Provider interface {
	// RegisterCluster retrieves cluster registration data needed to correctly identify the cluster.
	RegisterCluster(ctx context.Context, client castclient.Client) (*ClusterRegistration, error)
	// IsSpot checks provider specific properties whether the node lifecycle is spot/preemtible.
	IsSpot(ctx context.Context, node *v1.Node) (bool, error)
	// Name of the provider.
	Name() string
}

// ClusterRegistration holds information needed to identify the cluster.
type ClusterRegistration struct {
	ClusterID      string
	OrganizationID string
}

func (c *ClusterRegistration) String() string {
	return fmt.Sprintf("ClusterID=%q OrganizationID=%q", c.ClusterID, c.OrganizationID)
}
