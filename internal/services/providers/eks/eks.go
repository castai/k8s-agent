package eks

import (
	"context"
	"fmt"

	"github.com/patrickmn/go-cache"
	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/providers/eks/client"
	"castai-agent/internal/services/providers/types"
	"castai-agent/pkg/labels"
)

const (
	Name = "eks"

	LabelCapacity     = "eks.amazonaws.com/capacityType"
	ValueCapacitySpot = "SPOT"
)

// New configures and returns an EKS provider.
func New(ctx context.Context, log logrus.FieldLogger) (types.Provider, error) {
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

func (p *Provider) RegisterCluster(ctx context.Context, client castai.Client) (*types.ClusterRegistration, error) {
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

	req := &castai.RegisterClusterRequest{
		Name: *cn,
		EKS: castai.EKSParams{
			ClusterName: *cn,
			Region:      *r,
			AccountID:   *accID,
		},
	}

	resp, err := client.RegisterCluster(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("requesting castai api: %w", err)
	}

	return &types.ClusterRegistration{
		ClusterID:      resp.ID,
		OrganizationID: resp.OrganizationID,
	}, nil
}

func (p *Provider) IsSpot(ctx context.Context, node *v1.Node) (bool, error) {
	if val, ok := node.Labels[LabelCapacity]; ok && ValueCapacitySpot == val {
		return true, nil
	}

	if val, ok := node.Labels[labels.Spot]; ok && val == "true" {
		return true, nil
	}

	hostname, ok := node.Labels[v1.LabelHostname]
	if !ok {
		return false, fmt.Errorf("label %s not found on node %s", v1.LabelHostname, node.Name)
	}

	instances, err := p.awsClient.GetInstancesByPrivateDNS(ctx, []string{hostname})
	if err != nil {
		return false, fmt.Errorf("getting instances by hostname: %w", err)
	}

	for _, instance := range instances {
		if instance.InstanceLifecycle != nil && *instance.InstanceLifecycle == "spot" {
			return true, nil
		}
	}

	return false, nil
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

func (p *Provider) RegisterClusterRequest(ctx context.Context) (*castai.RegisterClusterRequest, error) {
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
	return &castai.RegisterClusterRequest{
		Name: *cn,
		EKS: castai.EKSParams{
			ClusterName: *cn,
			Region:      *r,
			AccountID:   *accID,
		},
	}, nil
}
