package eks

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/providers/eks/client"
	"castai-agent/internal/services/providers/types"
	"castai-agent/pkg/labels"
)

type nodeLifecycle string

const (
	NodeLifecycleUnknown  nodeLifecycle = "unknown"
	NodeLifecycleSpot     nodeLifecycle = "spot"
	NodeLifecycleOnDemand nodeLifecycle = "on_demand"
)

const (
	Name = "eks"

	LabelCapacity         = "eks.amazonaws.com/capacityType"
	ValueCapacitySpot     = "SPOT"
	ValueCapacityOnDemand = "ON_DEMAND"
)

// New configures and returns an EKS provider.
func New(ctx context.Context, log logrus.FieldLogger, apiNodeLifecycleDiscoveryEnabled bool, selfHosted bool) (types.Provider, error) {
	var opts []client.Opt

	if cfg := config.Get().EKS; cfg != nil {
		opts = append(opts, client.WithMetadata(cfg.AccountID, cfg.Region, cfg.ClusterName))
		opts = append(opts, client.WithAPITimeout(cfg.APITimeout))
	} else {
		opts = append(opts, client.WithMetadataDiscovery())
	}

	opts = append(opts, client.WithEC2Client())

	awsClient, err := client.New(ctx, log, opts...)
	if err != nil {
		return nil, fmt.Errorf("configuring aws client: %w", err)
	}

	return &Provider{
		log:                              log,
		awsClient:                        awsClient,
		apiNodeLifecycleDiscoveryEnabled: apiNodeLifecycleDiscoveryEnabled,
		spotCache:                        map[string]bool{},
		selfHosted:                       selfHosted,
	}, nil
}

// Provider is the EKS implementation of the providers.Provider interface.
type Provider struct {
	log                              logrus.FieldLogger
	awsClient                        client.Client
	apiNodeLifecycleDiscoveryEnabled bool
	selfHosted                       bool

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
	}

	if p.selfHosted {
		req.SelfHostedWithEC2Nodes = &castai.SelfHostedWithEC2NodesParams{
			ClusterName: *cn,
			Region:      *r,
			AccountID:   *accID,
		}
	} else {
		req.EKS = &castai.EKSParams{
			ClusterName: *cn,
			Region:      *r,
			AccountID:   *accID,
		}
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

	var instanceIDs []string
	nodesByInstanceID := map[string]*v1.Node{}

	for _, node := range nodes {
		lifecycle := determineLifecycle(node)
		if lifecycle == NodeLifecycleSpot {
			ret = append(ret, node)
			continue
		} else if lifecycle == NodeLifecycleOnDemand {
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

	if len(instanceIDs) == 0 || !p.apiNodeLifecycleDiscoveryEnabled {
		return ret, nil
	}

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

	return ret, nil
}

func (p *Provider) Name() string {
	return Name
}

func determineLifecycle(n *v1.Node) nodeLifecycle {
	if val, ok := n.Labels[LabelCapacity]; ok {
		if val == ValueCapacitySpot {
			return NodeLifecycleSpot
		} else if val == ValueCapacityOnDemand {
			return NodeLifecycleOnDemand
		}
	}

	if val, ok := n.Labels[labels.CastaiSpotFallback]; ok && val == "true" {
		return NodeLifecycleOnDemand
	}

	if val, ok := n.Labels[labels.CastaiSpot]; ok && val == "true" {
		return NodeLifecycleSpot
	}

	if val, ok := n.Labels[labels.WorkerSpot]; ok && val == "true" {
		return NodeLifecycleSpot
	}

	if val, ok := n.Labels[labels.KarpenterCapacityType]; ok {
		if val == labels.ValueKarpenterCapacityTypeSpot {
			return NodeLifecycleSpot
		} else if val == labels.ValueKarpenterCapacityTypeOnDemand {
			return NodeLifecycleOnDemand
		}
	}

	return NodeLifecycleUnknown
}
