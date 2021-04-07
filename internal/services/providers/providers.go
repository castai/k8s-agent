//go:generate mockgen -destination ./mock/provider.go . Provider
package providers

import (
	"castai-agent/internal/cast"
	"castai-agent/internal/config"
	"castai-agent/internal/services/providers/eks"
	"context"
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
)

// Provider is an abstraction for various CAST AI supported K8s providers, like EKS, GKE, etc.
type Provider interface {
	// RegisterClusterRequest retrieves all the required data needed to correctly register the cluster with CAST AI.
	RegisterClusterRequest(ctx context.Context) (*cast.RegisterClusterRequest, error)
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

func GetProvider(ctx context.Context, log logrus.FieldLogger) (p Provider, err error) {
	cfg := config.Get()
	switch cfg.Provider {
	case "":
		return dynamicProvider(ctx, log)
	case eks.Name:
		return eks.New(ctx, log)
	default:
		return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
	}
}

func dynamicProvider(ctx context.Context, log logrus.FieldLogger) (Provider, error) {
	if config.Get().EKS != nil {
		return eks.New(ctx, log)
	}

	log.Info("using cluster provider discovery")

	if p, err := eks.New(ctx, log); err != nil {
		log.Info(fmt.Errorf("eks did not succeed in discovery: %v", err))
	} else {
		return p, nil
	}

	return nil, errors.New("none of the supported providers were able to initialize")
}
