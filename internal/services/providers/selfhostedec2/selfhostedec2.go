//go:generate mockgen -destination ./mock/aws.go . IMDSClient,EC2Client
package selfhostedec2

import (
	"context"
	"fmt"
	"io"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/providers/types"
	"castai-agent/pkg/labels"
)

const Name = "selfhostedec2"

type IMDSClient interface {
	GetInstanceIdentityDocument(ctx context.Context, params *imds.GetInstanceIdentityDocumentInput, optFns ...func(*imds.Options)) (*imds.GetInstanceIdentityDocumentOutput, error)
	GetMetadata(ctx context.Context, params *imds.GetMetadataInput, optFns ...func(*imds.Options)) (*imds.GetMetadataOutput, error)
}

type EC2Client interface {
	DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
}

type Provider struct {
	log  logrus.FieldLogger
	imds IMDSClient
	ec2  EC2Client

	clusterName string
	region      string
	accountID   string

	spotCache                        map[string]bool
	apiNodeLifecycleDiscoveryEnabled bool
}

func New(ctx context.Context, log logrus.FieldLogger, cfg config.Config) (*Provider, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading aws config: %w", err)
	}

	imdsClient := imds.NewFromConfig(awsCfg)
	ec2Client := ec2.NewFromConfig(awsCfg)

	return &Provider{
		log:  log.WithField("provider", Name),
		imds: imdsClient,
		ec2:  ec2Client,

		clusterName: cfg.SelfHostedEC2.ClusterName,
		region:      cfg.SelfHostedEC2.Region,
		accountID:   cfg.SelfHostedEC2.AccountID,

		spotCache:                        make(map[string]bool),
		apiNodeLifecycleDiscoveryEnabled: cfg.SelfHostedEC2.APINodeLifecycleDiscoveryEnabled,
	}, nil
}

func (p *Provider) Name() string {
	return Name
}

func (p *Provider) RegisterCluster(ctx context.Context, castaiClient castai.Client) (*types.ClusterRegistration, error) {
	if p.region == "" || p.accountID == "" {
		iiDoc, err := p.imds.GetInstanceIdentityDocument(ctx, &imds.GetInstanceIdentityDocumentInput{})
		if err != nil {
			return nil, fmt.Errorf("getting instance identity document: %w", err)
		}

		if p.region == "" {
			p.region = iiDoc.Region
		}
		if p.accountID == "" {
			p.accountID = iiDoc.AccountID
		}
	}

	if p.clusterName == "" {
		clusterNameOutput, err := p.imds.GetMetadata(ctx, &imds.GetMetadataInput{
			Path: "tags/instance/castai:cluster-name",
		})
		if err != nil {
			return nil, fmt.Errorf("getting cluster name: %w", err)
		}

		clusterNameData, err := io.ReadAll(clusterNameOutput.Content)
		if err != nil {
			return nil, fmt.Errorf("reading cluster name data: %w", err)
		}

		p.clusterName = string(clusterNameData)
	}

	if p.region == "" {
		return nil, fmt.Errorf("region is empty")
	}

	if p.accountID == "" {
		return nil, fmt.Errorf("account id is empty")
	}

	if p.clusterName == "" {
		return nil, fmt.Errorf("cluster name is empty")
	}

	req := &castai.RegisterClusterRequest{
		Name: p.clusterName,
		SelfHostedWithEC2Nodes: &castai.AWSParams{
			ClusterName: p.clusterName,
			Region:      p.region,
			AccountID:   p.accountID,
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

	instanceIDsToCheck := make([]string, 0)
	nodesByInstanceID := map[string]*v1.Node{}

	for _, node := range nodes {
		lifecycle := determineLifecycle(node)
		if lifecycle == NodeLifecycleSpot {
			ret = append(ret, node)
			continue
		}
		if lifecycle == NodeLifecycleOnDemand {
			continue
		}

		splitProviderID := strings.Split(node.Spec.ProviderID, "/")
		instanceID := splitProviderID[len(splitProviderID)-1]
		if instanceID == "" {
			continue
		}
		spot, ok := p.spotCache[instanceID]
		if ok {
			if spot {
				ret = append(ret, node)
			}
			continue
		}

		instanceIDsToCheck = append(instanceIDsToCheck, instanceID)
		nodesByInstanceID[instanceID] = node
	}

	if len(instanceIDsToCheck) == 0 || !p.apiNodeLifecycleDiscoveryEnabled {
		return ret, nil
	}

	instances, err := p.GetInstances(ctx, instanceIDsToCheck)
	if err != nil {
		return nil, fmt.Errorf("getting instances: %w", err)
	}

	for _, instance := range instances {
		isSpot := instance.InstanceLifecycle == ec2types.InstanceLifecycleTypeSpot
		instanceID := *instance.InstanceId
		p.spotCache[instanceID] = isSpot

		if isSpot {
			ret = append(ret, nodesByInstanceID[instanceID])
		}
	}

	return ret, nil
}

func (p *Provider) GetInstances(ctx context.Context, instanceIDs []string) ([]ec2types.Instance, error) {
	var token *string
	var instances []ec2types.Instance

	for {
		resp, err := p.ec2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			NextToken:   token,
			InstanceIds: instanceIDs,
		})
		if err != nil {
			return nil, fmt.Errorf("describing instances: %w", err)
		}

		for _, reservation := range resp.Reservations {
			instances = append(instances, reservation.Instances...)
		}

		if resp.NextToken == nil {
			break
		}
	}

	return instances, nil
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

	return NodeLifecycleUnknown
}
