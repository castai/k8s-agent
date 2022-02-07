package eks

import (
	"context"
	"fmt"
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

		if val, ok := n.Labels[labels.CastaiSpot]; ok && val == "true" {
			return true
		}

		return false
	}

	var hostnames []string
	nodesByHostname := map[string]*v1.Node{}

	for _, node := range nodes {
		if isLabeledSpot(node) {
			ret = append(ret, node)
			continue
		}

		hostname, ok := node.Labels[v1.LabelHostname]
		if !ok {
			return nil, fmt.Errorf("label %s not found on node %s", v1.LabelHostname, node.Name)
		}

		spot, ok := p.spotCache[hostname]
		if ok {
			if spot {
				ret = append(ret, node)
			}
			continue
		}

		hostnames = append(hostnames, hostname)
		nodesByHostname[hostname] = node
	}

	if len(hostnames) > 0 {
		instances, err := p.awsClient.GetInstancesByPrivateDNS(ctx, hostnames)
		if err != nil {
			return nil, fmt.Errorf("getting instances by private dns: %w", err)
		}

		for _, instance := range instances {
			isSpot := instance.InstanceLifecycle != nil && *instance.InstanceLifecycle == "spot"
			hostname := *instance.PrivateDnsName

			if isSpot {
				ret = append(ret, nodesByHostname[hostname])
			}

			p.spotCache[hostname] = isSpot
		}
	}

	return ret, nil
}

func (p *Provider) Name() string {
	return Name
}
