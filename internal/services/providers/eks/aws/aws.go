//go:generate mockgen -destination ./mock/aws.go . RegisterClusterBuilder,Client
package aws

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"

	"castai-agent/internal/castai"
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
	LabelCapacity         = "eks.amazonaws.com/capacityType"
	ValueCapacitySpot     = "SPOT"
	ValueCapacityOnDemand = "ON_DEMAND"
)

type RegisterClusterBuilder interface {
	BuildRegisterClusterRequest(ctx context.Context) (*castai.RegisterClusterRequest, error)
}

func NewProvider(log logrus.FieldLogger, name string, awsClient Client, apiNodeLifecycleDiscoveryEnabled bool, registerClusterBuilder RegisterClusterBuilder) (types.Provider, error) {
	return &Provider{
		name:                             name,
		log:                              log,
		awsClient:                        awsClient,
		apiNodeLifecycleDiscoveryEnabled: apiNodeLifecycleDiscoveryEnabled,
		spotCache:                        map[string]bool{},
		registerClusterBuilder:           registerClusterBuilder,
	}, nil
}

type Provider struct {
	name                             string
	log                              logrus.FieldLogger
	awsClient                        Client
	apiNodeLifecycleDiscoveryEnabled bool
	registerClusterBuilder           RegisterClusterBuilder
	spotCache                        map[string]bool
}

func (p *Provider) RegisterCluster(ctx context.Context, client castai.Client) (*types.ClusterRegistration, error) {
	req, err := p.registerClusterBuilder.BuildRegisterClusterRequest(ctx)
	if err != nil {
		return nil, fmt.Errorf("building register cluster request: %w", err)
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
	return p.name
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

// Client is an abstraction on the AWS SDK to enable easier mocking and manipulation of request data.
type Client interface {
	// GetRegion returns the AWS EC2 instance region. It can be discovered by using WithMetadataDiscovery opt, which
	// will dynamically get the region from the instance metadata endpoint. Or, it can be overriden by setting the
	// environment variable EKS_REGION.
	GetRegion(ctx context.Context) (*string, error)
	// GetAccountID returns the AWS EC2 instance account ID. It can be discovered by using WithMetadataDiscovery opt,
	// which will dynamically get the account ID from the instance metadata endpoint. Or, it can be overriden by setting
	// the environment variable EKS_ACCOUNT_ID.
	GetAccountID(ctx context.Context) (*string, error)
	// GetClusterName returns the AWS EKS cluster name. It can be discovered dynamically by using WithMetadataDiscovery
	// opt, which will dynamically get the cluster name by doing a combination of metadata and EC2 SDK calls. Or, it can
	// be overridden by setting the environment variable EKS_CLUSTER_NAME.
	GetClusterName(ctx context.Context) (*string, error)
	// GetInstancesByInstanceIDs returns a list of EC2 instances from the EC2 SDK by filtering on the instance IDs which
	// can be retrieved from node.spec.providerID.
	GetInstancesByInstanceIDs(ctx context.Context, instanceIDs []string) ([]*ec2.Instance, error)
}

// New creates and configures a new AWS Client instance.
func New(ctx context.Context, log logrus.FieldLogger, opts ...Opt) (Client, error) {
	c := &client{log: log}

	for _, opt := range opts {
		if err := opt(ctx, c); err != nil {
			return nil, err
		}
	}

	return c, nil
}

const (
	tagEKSK8sCluster         = "k8s.io/cluster/"
	tagEKSKubernetesCluster  = "kubernetes.io/cluster/"
	tagKOPSKubernetesCluster = "KubernetesCluster"
	owned                    = "owned"
)

var (
	eksClusterTags = []string{
		tagEKSK8sCluster,
		tagEKSKubernetesCluster,
	}
)

// Opt for configuring the AWS Client.
type Opt func(ctx context.Context, c *client) error

// WithEC2Client configures an EC2 SDK client. AWS region must be already discovered or set on an environment variable.
func WithEC2Client() func(ctx context.Context, c *client) error {
	return func(ctx context.Context, c *client) error {
		sess, err := session.NewSession(aws.
			NewConfig().
			WithRegion(*c.region).
			WithCredentialsChainVerboseErrors(true))
		if err != nil {
			return fmt.Errorf("creating aws sdk session: %w", err)
		}

		c.sess = sess
		c.ec2Client = ec2.New(sess)

		return nil
	}
}

// WithValidateCredentials validates the aws-sdk credentials chain.
func WithValidateCredentials() func(ctx context.Context, c *client) error {
	return func(ctx context.Context, c *client) error {
		if _, err := c.sess.Config.Credentials.Get(); err != nil {
			return fmt.Errorf("validating aws credentials: %w", err)
		}
		return nil
	}
}

// WithMetadata configures the discoverable EC2 instance metadata and EKS properties by setting static values instead
// of relying on the discovery mechanism.
func WithMetadata(accountID, region, clusterName string) func(ctx context.Context, c *client) error {
	return func(ctx context.Context, c *client) error {
		c.accountID = &accountID
		c.region = &region
		c.clusterName = &clusterName
		return nil
	}
}

// WithMetadataDiscovery configures the EC2 instance metadata client to enable dynamic discovery of those properties.
func WithMetadataDiscovery() func(ctx context.Context, c *client) error {
	return func(ctx context.Context, c *client) error {
		metaSess, err := session.NewSession()
		if err != nil {
			return fmt.Errorf("creating metadata session: %w", err)
		}

		c.metaClient = ec2metadata.New(metaSess)

		region, err := c.metaClient.RegionWithContext(ctx)
		if err != nil {
			return fmt.Errorf("getting instance region: %w", err)
		}

		c.region = &region

		return nil
	}
}

// WithAPITimeout configures the request timeout for AWS API calls.
func WithAPITimeout(apiTimeout time.Duration) func(ctx context.Context, c *client) error {
	return func(ctx context.Context, c *client) error {
		c.APITimeout = apiTimeout

		return nil
	}
}

type client struct {
	log         logrus.FieldLogger
	sess        *session.Session
	metaClient  *ec2metadata.EC2Metadata
	ec2Client   *ec2.EC2
	region      *string
	accountID   *string
	clusterName *string
	APITimeout  time.Duration
}

func (c *client) GetRegion(ctx context.Context) (*string, error) {
	if c.region != nil {
		return c.region, nil
	}

	region, err := c.metaClient.RegionWithContext(ctx)
	if err != nil {
		return nil, err
	}

	c.region = &region

	return c.region, nil
}

func (c *client) GetAccountID(ctx context.Context) (*string, error) {
	if c.accountID != nil {
		return c.accountID, nil
	}
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	c.log.Debug("fetching instance identity document")
	resp, err := c.metaClient.GetInstanceIdentityDocumentWithContext(ctx)
	if err != nil {
		return nil, err
	}

	c.accountID = &resp.AccountID

	return c.accountID, nil
}

func (c *client) GetClusterName(ctx context.Context) (*string, error) {
	if c.clusterName != nil {
		return c.clusterName, nil
	}

	instanceID, err := c.getMetadataWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting instance id from metadata: %w", err)
	}

	resp, err := c.describeInstancesWithContext(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []*string{pointer.StringPtr(instanceID)},
	})
	if err != nil {
		return nil, fmt.Errorf("describing instance_id=%s: %w", instanceID, err)
	}

	if len(resp.Reservations) == 0 {
		return nil, fmt.Errorf("no reservations found for instance_id=%s", instanceID)
	}

	if len(resp.Reservations[0].Instances) == 0 {
		return nil, fmt.Errorf("no instances found for instance_id=%s", instanceID)
	}

	if len(resp.Reservations[0].Instances[0].Tags) == 0 {
		return nil, fmt.Errorf("no tags found for instance_id=%s", instanceID)
	}

	clusterName := getClusterName(resp.Reservations[0].Instances[0].Tags)
	if clusterName == "" {
		return nil, fmt.Errorf("discovering cluster name: instance cluster tags not found for instance_id=%s", instanceID)
	}

	c.clusterName = &clusterName

	return c.clusterName, nil
}

func (c *client) GetInstancesByInstanceIDs(ctx context.Context, instanceIDs []string) ([]*ec2.Instance, error) {
	idsPtr := make([]*string, len(instanceIDs))
	for i := range instanceIDs {
		idsPtr[i] = &instanceIDs[i]
	}

	var instances []*ec2.Instance

	batchSize := 20
	for i := 0; i < len(idsPtr); i += batchSize {
		batch := idsPtr[i:int(math.Min(float64(i+batchSize), float64(len(idsPtr))))]

		req := &ec2.DescribeInstancesInput{
			Filters: []*ec2.Filter{
				{
					Name:   pointer.StringPtr("instance-id"),
					Values: batch,
				},
			},
		}

		resp, err := c.describeInstancesWithContext(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("describing instances: %w", err)
		}

		for _, reservation := range resp.Reservations {
			instances = append(instances, reservation.Instances...)
		}
	}

	return instances, nil
}

func (c *client) getMetadataWithContext(ctx context.Context) (string, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	c.log.Debug("fetching EC2 instance metadata")
	return c.metaClient.GetMetadataWithContext(ctx, "instance-id")
}

func (c *client) describeInstancesWithContext(ctx context.Context, req *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
	ctxWithTimeout, cancel := c.requestContext(ctx)
	defer cancel()

	c.log.Debug("fetching EC2 instance information")
	return c.ec2Client.DescribeInstancesWithContext(ctxWithTimeout, req)
}

func (c *client) requestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.APITimeout > 0 {
		return context.WithTimeout(ctx, c.APITimeout)
	}

	return ctx, func() {}
}

func getClusterName(tags []*ec2.Tag) string {
	for _, tag := range tags {
		if tag == nil || tag.Key == nil || tag.Value == nil {
			continue
		}
		for _, clusterTag := range eksClusterTags {
			if strings.HasPrefix(*tag.Key, clusterTag) && *tag.Value == owned {
				return strings.TrimPrefix(*tag.Key, clusterTag)
			}
		}
		if *tag.Key == tagKOPSKubernetesCluster {
			return *tag.Value
		}
	}
	return ""
}
