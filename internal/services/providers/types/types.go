//go:generate mockgen -destination ./mock/provider.go . Provider
package types

import (
	"context"

	v1 "k8s.io/api/core/v1"

	castclient "castai-agent/internal/castai"
)

// Provider is an abstraction for various CAST AI supported K8s providers, like EKS, GKE, etc.
type Provider interface {
	// RegisterCluster retrieves cluster registration data needed to correctly identify the cluster.
	RegisterCluster(ctx context.Context, client castclient.Client) (*ClusterRegistration, error)
	// FilterSpot returns a list of nodes which are configured as spot/preemtible instances.
	FilterSpot(ctx context.Context, nodes []*v1.Node) ([]*v1.Node, error)
	// Name of the provider.
	Name() string
	// AccountID of the EC2 instance.
	// Deprecated: snapshot should not include cluster metadata as it already is known via register cluster request.
	AccountID(ctx context.Context) (string, error)
	// ClusterName of the of the EKS cluster.
	// Deprecated: snapshot should not include cluster name as it already is known via register cluster request.
	ClusterName(ctx context.Context) (string, error)
	// ClusterRegion of the EC2 instance.
	// Deprecated: snapshot should not include cluster metadata as it already is known via register cluster request.
	ClusterRegion(ctx context.Context) (string, error)
}

// ClusterRegistration holds information needed to identify the cluster.
type ClusterRegistration struct {
	ClusterID      string
	OrganizationID string
}
