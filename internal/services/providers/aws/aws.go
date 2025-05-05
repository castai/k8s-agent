package aws

import (
	"context"
	"fmt"
	"io"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/providers/types"
	"castai-agent/pkg/labels"
)

const Name = "aws"

type Provider struct {
	log  logrus.FieldLogger
	imds *imds.Client
}

func New(ctx context.Context, log logrus.FieldLogger) (*Provider, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading aws config: %w", err)
	}

	imdsClient := imds.NewFromConfig(awsCfg)

	return &Provider{
		log:  log.WithField("provider", Name),
		imds: imdsClient,
	}, nil
}

func (p *Provider) Name() string {
	return Name
}

func (p *Provider) RegisterCluster(ctx context.Context, castaiClient castai.Client) (*types.ClusterRegistration, error) {
	cfg := config.Get().AWS

	var (
		region      = ""
		accountID   = ""
		clusterName = ""
	)
	if cfg != nil {
		region = cfg.Region
		accountID = cfg.AccountID
		clusterName = cfg.ClusterName
	}

	if region == "" || accountID == "" {
		iiDoc, err := p.imds.GetInstanceIdentityDocument(ctx, &imds.GetInstanceIdentityDocumentInput{})
		if err != nil {
			return nil, fmt.Errorf("getting instance identity document: %w", err)
		}

		if region == "" {
			region = iiDoc.Region
		}
		if accountID == "" {
			accountID = iiDoc.AccountID
		}
	}

	if clusterName == "" {
		clusterNameOutput, err := p.imds.GetMetadata(ctx, &imds.GetMetadataInput{
			Path: "tags/instance/cluster-name",
		})
		if err != nil {
			return nil, fmt.Errorf("getting cluster name: %w", err)
		}

		clusterNameData, err := io.ReadAll(clusterNameOutput.Content)
		if err != nil {
			return nil, fmt.Errorf("reading cluster name data: %w", err)
		}

		clusterName = string(clusterNameData)
	}

	if region == "" {
		return nil, fmt.Errorf("region is empty")
	}

	if accountID == "" {
		return nil, fmt.Errorf("account id is empty")
	}

	if clusterName == "" {
		return nil, fmt.Errorf("cluster name is empty")
	}

	req := &castai.RegisterClusterRequest{
		Name: clusterName,
		AWS: &castai.AWSParams{
			ClusterName: clusterName,
			Region:      region,
			AccountID:   accountID,
		},
	}

	resp, err := castaiClient.RegisterCluster(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("requesting castai api: %w", err)
	}

	return &types.ClusterRegistration{
		ClusterID:      resp.Cluster.ID,
		OrganizationID: resp.Cluster.OrganizationID,
	}, nil
}

func (p *Provider) FilterSpot(ctx context.Context, nodes []*v1.Node) ([]*v1.Node, error) {
	var ret []*v1.Node

	for _, node := range nodes {
		lifecycle := determineLifecycle(node)
		if lifecycle == NodeLifecycleSpot {
			ret = append(ret, node)
		}
	}

	return ret, nil
}

type NodeLifecycle string

const (
	NodeLifecycleUnknown  NodeLifecycle = "unknown"
	NodeLifecycleOnDemand NodeLifecycle = "on-demand"
	NodeLifecycleSpot     NodeLifecycle = "spot"
)

func determineLifecycle(node *v1.Node) NodeLifecycle {
	if node.Labels == nil {
		return NodeLifecycleUnknown
	}

	if val, ok := node.Labels[labels.CastaiSpot]; ok && val == "true" {
		return NodeLifecycleSpot
	}

	val, ok := node.Labels[labels.KarpenterCapacityType]
	if ok {
		if val == labels.ValueKarpenterCapacityTypeSpot {
			return NodeLifecycleSpot
		}
		if val == labels.ValueKarpenterCapacityTypeOnDemand {
			return NodeLifecycleOnDemand
		}
	}

	// TODO: might need to improve this and call also the AWS EC2 API
	return NodeLifecycleOnDemand
}
