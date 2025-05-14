package kops

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/discovery"
	awsclient "castai-agent/internal/services/providers/eks/aws"
	"castai-agent/internal/services/providers/gke"
	"castai-agent/internal/services/providers/types"
	"castai-agent/pkg/cloud"
	"castai-agent/pkg/labels"
)

const Name = "kops"

func New(log logrus.FieldLogger, discoveryService discovery.Service) (types.Provider, error) {
	return &Provider{
		log:              log,
		discoveryService: discoveryService,
	}, nil
}

type Provider struct {
	log              logrus.FieldLogger
	discoveryService discovery.Service
	awsClient        awsclient.Client
	csp              cloud.Cloud
}

func (p *Provider) RegisterCluster(ctx context.Context, client castai.Client) (*types.ClusterRegistration, error) {
	clusterID, err := p.discoveryService.GetKubeSystemNamespaceID(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting cluster ID: %w", err)
	}

	req := &castai.RegisterClusterRequest{
		ID:   *clusterID,
		KOPS: &castai.KOPSParams{},
	}

	if cfg := config.Get().KOPS; cfg != nil {
		req.Name = cfg.ClusterName
		req.KOPS.CSP = cfg.CSP
		req.KOPS.Region = cfg.Region
		req.KOPS.ClusterName = cfg.ClusterName
		req.KOPS.StateStore = cfg.StateStore
	} else {
		csp, region, err := p.discoveryService.GetCSPAndRegion(ctx)
		if err != nil {
			return nil, fmt.Errorf("getting csp and region: %w", err)
		}

		clusterName, stateStore, err := p.discoveryService.GetKOPSClusterNameAndStateStore(ctx, p.log)
		if err != nil {
			return nil, fmt.Errorf("getting cluster name and state store: %w", err)
		}

		req.Name = clusterName
		req.KOPS.CSP = string(csp)
		req.KOPS.Region = region
		req.KOPS.ClusterName = clusterName
		req.KOPS.StateStore = stateStore
	}

	p.log.Infof("discovered kops cluster properties: %+v", req)

	p.csp = cloud.Cloud(req.KOPS.CSP)

	if p.csp == cloud.AWS {
		opts := []awsclient.Opt{
			awsclient.WithMetadata("", req.KOPS.Region, req.KOPS.ClusterName),
			awsclient.WithEC2Client(),
			awsclient.WithValidateCredentials(),
		}
		c, err := awsclient.New(ctx, p.log, opts...)
		if err != nil {
			p.log.Errorf(
				"failed initializing aws client, spot functionality for savings estimation will be reduced: %v",
				err,
			)
		} else {
			p.awsClient = c
		}
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

func (p *Provider) FilterSpot(ctx context.Context, nodes []*v1.Node) ([]*v1.Node, error) {
	var ret []*v1.Node

	for _, node := range nodes {
		spot, err := p.isSpot(ctx, node)
		if err != nil {
			return nil, fmt.Errorf("determining whether node %s is spot: %w", node.Name, err)
		}

		if spot {
			ret = append(ret, node)
		}
	}

	return ret, nil
}

func (p *Provider) isSpot(ctx context.Context, node *v1.Node) (bool, error) {
	if val, ok := node.Labels[labels.CastaiSpot]; ok && val == "true" {
		return true, nil
	}

	if val, ok := node.Labels[labels.KopsSpot]; ok && val == "true" {
		return true, nil
	}

	if p.csp == cloud.AWS && p.awsClient != nil {
		splitProviderID := strings.Split(node.Spec.ProviderID, "/")
		instanceID := splitProviderID[len(splitProviderID)-1]

		instances, err := p.awsClient.GetInstancesByInstanceIDs(ctx, []string{instanceID})
		if err != nil {
			return false, fmt.Errorf("getting instances by instance IDs: %w", err)
		}

		for _, instance := range instances {
			if instance.InstanceLifecycle != nil && *instance.InstanceLifecycle == "spot" {
				return true, nil
			}
		}
	}

	if p.csp == cloud.GCP {
		if val, ok := node.Labels[gke.LabelPreemptible]; ok && val == "true" {
			return true, nil
		}
	}

	return false, nil
}

func (p *Provider) Name() string {
	return Name
}
