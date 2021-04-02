package eks

import (
	"castai-agent/internal/cast"
	"castai-agent/internal/config"
	"castai-agent/internal/services/providers/eks/client"
	"context"
	"fmt"
	"github.com/patrickmn/go-cache"
	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"time"
)

const (
	Name = "eks"
)

// New configures and returns an EKS provider.
func New(ctx context.Context, log logrus.FieldLogger) (*Provider, error) {
	log = log.WithField("provider", Name)

	var opts []client.Opt

	if cfg := config.Get().EKS; cfg != nil {
		opts = append(opts, client.WithMetadata(cfg.AccountID, cfg.Region, cfg.ClusterName))
	} else {
		opts = append(opts, client.WithMetadataDiscovery())
	}

	opts = append(opts, client.WithEC2Client())

	awsClient, err := client.New(ctx, log, opts...)
	if err != nil {
		return nil, fmt.Errorf("configuring aws client: %w", err)
	}

	return &Provider{
		log:       log,
		awsClient: awsClient,
	}, nil
}

// Provider is the EKS implementation of the providers.Provider interface.
type Provider struct {
	log       logrus.FieldLogger
	awsClient client.Client
	spotCache *cache.Cache
}

func (p *Provider) FilterSpot(ctx context.Context, nodes []*v1.Node) ([]*v1.Node, error) {
	if p.spotCache == nil {
		p.spotCache = cache.New(60*time.Minute, 10*time.Minute)
	}

	var spotNodes []*v1.Node
	var checkPrivateDNS []string

	for _, node := range nodes {
		val, exists := p.spotCache.Get(node.Name)

		if !exists {
			checkPrivateDNS = append(checkPrivateDNS, node.ObjectMeta.Labels[v1.LabelHostname])
			continue
		}

		spot := val.(bool)

		if spot {
			spotNodes = append(spotNodes, node)
		}
	}

	if len(checkPrivateDNS) > 0 {
		instances, err := p.awsClient.GetInstancesByPrivateDNS(ctx, checkPrivateDNS)
		if err != nil {
			return nil, err
		}

		spotInstances := make(map[string]bool, len(instances))
		for _, instance := range instances {
			spotInstances[*instance.PrivateDnsName] = instance.InstanceLifecycle != nil && *instance.InstanceLifecycle == "spot"
		}

		for _, node := range nodes {
			spot, ok := spotInstances[node.ObjectMeta.Labels[v1.LabelHostname]]
			if ok {
				p.spotCache.SetDefault(node.Name, spot)
			}
			if spot {
				spotNodes = append(spotNodes, node)
			}
		}
	}

	return spotNodes, nil
}

func (p *Provider) Name() string {
	return Name
}

func (p *Provider) ClusterName(ctx context.Context) (string, error) {
	cn, err := p.awsClient.GetClusterName(ctx)
	if err != nil {
		return "", err
	}
	return *cn, nil
}

func (p *Provider) ClusterRegion(ctx context.Context) (string, error) {
	r, err := p.awsClient.GetRegion(ctx)
	if err != nil {
		return "", err
	}
	return *r, nil
}

func (p *Provider) AccountID(ctx context.Context) (string, error) {
	accID, err := p.awsClient.GetAccountID(ctx)
	if err != nil {
		return "", err
	}
	return *accID, nil
}

func (p *Provider) RegisterClusterRequest(ctx context.Context) (*cast.RegisterClusterRequest, error) {
	cn, err := p.awsClient.GetClusterName(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting cluster name: %w", err)
	}
	r, err := p.awsClient.GetRegion(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting region: %w", err)
	}
	accID, err := p.awsClient.GetAccountID(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting account id: %w", err)
	}
	return &cast.RegisterClusterRequest{
		Name: *cn,
		EKS: cast.EKSParams{
			ClusterName: *cn,
			Region:      *r,
			AccountID:   *accID,
		},
	}, nil
}
