package openshift

import (
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/discovery"
	"castai-agent/internal/services/providers/types"
)

const Name = "openshift"

var _ types.Provider = (*Provider)(nil)

type Provider struct {
	discoveryService discovery.Service
	dyno             dynamic.Interface
}

func New(discoveryService discovery.Service, dyno dynamic.Interface) *Provider {
	return &Provider{
		discoveryService: discoveryService,
		dyno:             dyno,
	}
}

func (p *Provider) RegisterCluster(ctx context.Context, client castai.Client) (*types.ClusterRegistration, error) {
	clusterID, err := p.discoveryService.GetClusterID(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting cluster id: %w", err)
	}

	var csp, region, clusterName, internalID string

	if cfg := config.Get().OpenShift; cfg != nil {
		csp, region, clusterName, internalID = cfg.CSP, cfg.Region, cfg.ClusterName, cfg.InternalID
	} else {
		c, r, err := p.discoveryService.GetCSPAndRegion(ctx)
		if err != nil {
			return nil, fmt.Errorf("getting csp and region: %w", err)
		}

		id, err := p.discoveryService.GetOpenshiftClusterID(ctx)
		if err != nil {
			return nil, fmt.Errorf("getting openshift cluster id: %w", err)
		}

		cn, err := p.discoveryService.GetOpenshiftClusterName(ctx)
		if err != nil {
			return nil, fmt.Errorf("getting openshift cluster name: %w", err)
		}

		csp, region, clusterName, internalID = *c, *r, *cn, *id
	}

	resp, err := client.RegisterCluster(ctx, &castai.RegisterClusterRequest{
		ID:   *clusterID,
		Name: clusterName,
		Openshift: &castai.OpenshiftParams{
			CSP:         csp,
			Region:      region,
			ClusterName: clusterName,
			InternalID:  internalID,
		},
	})
	if err != nil {
		return nil, err
	}

	return &types.ClusterRegistration{
		ClusterID:      resp.ID,
		OrganizationID: resp.OrganizationID,
	}, nil
}

func (p *Provider) Name() string {
	return Name
}

func (p *Provider) FilterSpot(ctx context.Context, nodes []*v1.Node) ([]*v1.Node, error) {
	var next string
	spotInstanceIDs := make(map[string]bool)

	for {
		machines, err := p.dyno.Resource(discovery.OpenshiftMachinesGVR).
			Namespace(discovery.OpenshiftMachineAPINamespace).
			List(ctx, metav1.ListOptions{Limit: 100, Continue: next})
		if err != nil {
			return nil, fmt.Errorf("listing machines: %w", err)
		}

		for _, machine := range machines.Items {
			_, ok, err := unstructured.NestedFieldNoCopy(machine.Object, "spec", "providerSpec", "value", "spotMarketOptions")
			if err != nil {
				return nil, fmt.Errorf("getting spotMarketOptions: %w", err)
			}
			if !ok {
				continue
			}

			instanceID, ok, err := unstructured.NestedString(machine.Object, "status", "providerStatus", "instanceId")
			if err != nil {
				return nil, fmt.Errorf("getting machine instance id: %w", err)
			}
			if !ok {
				continue
			}
			spotInstanceIDs[instanceID] = true
		}

		next = machines.GetContinue()
		if next == "" {
			break
		}
	}

	var filtered []*v1.Node

	for _, node := range nodes {
		splitProviderID := strings.Split(node.Spec.ProviderID, "/")
		instanceID := splitProviderID[len(splitProviderID)-1]

		if spotInstanceIDs[instanceID] {
			filtered = append(filtered, node)
		}
	}

	return filtered, nil
}
