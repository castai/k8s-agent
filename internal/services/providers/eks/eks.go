package eks

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"

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
		spotCache: map[string]bool{},
	}, nil
}

// Provider is the EKS implementation of the providers.Provider interface.
type Provider struct {
	log       logrus.FieldLogger
	awsClient client.Client

	spotCache map[string]bool
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
		EKS: &castai.EKSParams{
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

func (p *Provider) FilterSpot(ctx context.Context, nodes []*v1.Node) ([]*v1.Node, error) {
	var ret []*v1.Node

	isLabeledSpot := func(n *v1.Node) bool {
		if val, ok := n.Labels[LabelCapacity]; ok && val == ValueCapacitySpot {
			return true
		}

		if val, ok := n.Labels[labels.CastaiSpot]; ok && val == labels.ValueTrue {
			return true
		}

		return false
	}

	var instanceIDs []string
	nodesByInstanceID := map[string]*v1.Node{}

	for _, node := range nodes {
		if isLabeledSpot(node) {
			ret = append(ret, node)
			continue
		}

		splitProviderID := strings.Split(node.Spec.ProviderID, "/")
		instanceID := splitProviderID[len(splitProviderID)-1]

		spot, ok := p.spotCache[instanceID]
		if ok {
			if spot {
				ret = append(ret, node)
			}
			continue
		}

		instanceIDs = append(instanceIDs, instanceID)
		nodesByInstanceID[instanceID] = node
	}

	if len(instanceIDs) > 0 {
		instances, err := p.awsClient.GetInstancesByInstanceIDs(ctx, instanceIDs)
		if err != nil {
			return nil, fmt.Errorf("getting instances by instance IDs: %w", err)
		}

		for _, instance := range instances {
			isSpot := instance.InstanceLifecycle != nil && *instance.InstanceLifecycle == "spot"
			instanceID := *instance.InstanceId

			if isSpot {
				ret = append(ret, nodesByInstanceID[instanceID])
			}

			p.spotCache[instanceID] = isSpot
		}
	}

	return ret, nil
}

func (p *Provider) Name() string {
	return Name
}
