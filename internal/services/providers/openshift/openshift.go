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
	clusterID, err := p.discoveryService.GetKubeSystemNamespaceID(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting cluster id: %w", err)
	}

	req := &castai.RegisterClusterRequest{
		ID:        *clusterID,
		Openshift: &castai.OpenshiftParams{},
	}

	if cfg := config.Get().OpenShift; cfg != nil {
		req.Name = cfg.ClusterName
		req.Openshift.CSP = cfg.CSP
		req.Openshift.Region = cfg.Region
		req.Openshift.ClusterName = cfg.ClusterName
		req.Openshift.InternalID = cfg.InternalID
	} else {
		csp, region, err := p.discoveryService.GetCSPAndRegion(ctx)
		if err != nil {
			return nil, fmt.Errorf("getting csp and region: %w", err)
		}

		internalID, err := p.discoveryService.GetOpenshiftClusterID(ctx)
		if err != nil {
			return nil, fmt.Errorf("getting openshift cluster id: %w", err)
		}

		clusterName, err := p.discoveryService.GetOpenshiftClusterName(ctx)
		if err != nil {
			return nil, fmt.Errorf("getting openshift cluster name: %w", err)
		}

		req.Name = clusterName
		req.Openshift.CSP = string(csp)
		req.Openshift.Region = region
		req.Openshift.ClusterName = clusterName
		req.Openshift.InternalID = internalID
	}

	resp, err := client.RegisterCluster(ctx, req)
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
			// If the spotMarketOptions field exists, then this is a spot instance.
			// https://docs.openshift.com/container-platform/4.10/machine_management/creating_machinesets/creating-machineset-aws.html#machineset-creating-non-guaranteed-instance_creating-machineset-aws
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
