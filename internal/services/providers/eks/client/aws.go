//go:generate mockgen -destination ./mock/client.go . Client
package client

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/sirupsen/logrus"
	"k8s.io/utils/pointer"
)

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
	// be overriden by setting the environment variable EKS_CLUSTER_NAME.
	GetClusterName(ctx context.Context) (*string, error)
	// GetInstancesByPrivateDNS returns a list of EC2 instances from the EC2 SDK by filtering on the private DNS which
	// can be retrieved from K8s node labels.
	GetInstancesByPrivateDNS(ctx context.Context, dns []string) ([]*ec2.Instance, error)
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
	tagK8sCluster        = "k8s.io/cluster/"
	tagKubernetesCluster = "kubernetes.io/cluster/"
	owned                = "owned"
)

var (
	clusterTags = []string{
		tagK8sCluster,
		tagKubernetesCluster,
	}
)

// Opt for configuring the AWS Client.
type Opt func(ctx context.Context, c *client) error

// WithEC2Client configures an EC2 SDK client. AWS region must be already discovered or set on an environment variable.
func WithEC2Client() func(ctx context.Context, c *client) error {
	return func(ctx context.Context, c *client) error {
		sess, err := session.NewSession(aws.NewConfig().WithRegion(*c.region))
		if err != nil {
			return fmt.Errorf("creating aws sdk session: %w", err)
		}

		c.ec2Client = ec2.New(sess)

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

type client struct {
	log         logrus.FieldLogger
	metaClient  *ec2metadata.EC2Metadata
	ec2Client   *ec2.EC2
	region      *string
	accountID   *string
	clusterName *string
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

	instanceID, err := c.metaClient.GetMetadataWithContext(ctx, "instance-id")
	if err != nil {
		return nil, err
	}

	req := &ec2.DescribeTagsInput{
		Filters: []*ec2.Filter{
			{
				Name:   pointer.StringPtr("resource-type"),
				Values: []*string{pointer.StringPtr("instance")},
			},
			{
				Name:   pointer.StringPtr("resource-id"),
				Values: []*string{&instanceID},
			},
		},
	}

	var clusterName string

	if err := c.ec2Client.DescribeTagsPagesWithContext(ctx, req, func(page *ec2.DescribeTagsOutput, last bool) bool {
		for _, tag := range page.Tags {
			if tag.Key == nil || tag.Value == nil {
				continue
			}
			for _, clusterTag := range clusterTags {
				if strings.HasPrefix(*tag.Key, clusterTag) && *tag.Value == owned {
					clusterName = strings.TrimPrefix(*tag.Key, clusterTag)
					return false
				}
			}
		}
		return last
	}); err != nil {
		return nil, err
	}

	if clusterName == "" {
		return nil, errors.New("discovering cluster name: instance cluster tags not found")
	}

	c.clusterName = &clusterName

	return c.clusterName, nil
}

func (c *client) GetInstancesByPrivateDNS(ctx context.Context, dns []string) ([]*ec2.Instance, error) {
	dnsPtr := make([]*string, len(dns))
	for i := range dns {
		dnsPtr[i] = &dns[i]
	}

	var instances []*ec2.Instance

	batchSize := 20
	for i := 0; i < len(dnsPtr); i += batchSize {
		batch := dnsPtr[i:int(math.Min(float64(i+batchSize), float64(len(dnsPtr))))]

		req := &ec2.DescribeInstancesInput{
			Filters: []*ec2.Filter{
				{
					Name:   pointer.StringPtr("private-dns-name"),
					Values: batch,
				},
			},
		}

		resp, err := c.ec2Client.DescribeInstancesWithContext(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("describing instances: %w", err)
		}

		for _, reservation := range resp.Reservations {
			instances = append(instances, reservation.Instances...)
		}
	}

	return instances, nil
}
