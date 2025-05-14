package eks

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/providers/eks/aws"
	"castai-agent/internal/services/providers/types"
)

const Name = "eks"

type registerClusterBuilder struct {
	awsClient aws.Client
}

func newRegisterClusterBuilder(awsClient aws.Client) *registerClusterBuilder {
	return &registerClusterBuilder{
		awsClient: awsClient,
	}
}

func (b *registerClusterBuilder) BuildRegisterClusterRequest(ctx context.Context) (*castai.RegisterClusterRequest, error) {
	cn, err := b.awsClient.GetClusterName(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting cluster name: %w", err)
	}
	r, err := b.awsClient.GetRegion(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting region: %w", err)
	}
	accID, err := b.awsClient.GetAccountID(ctx)
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

	return req, nil
}

func New(ctx context.Context, log logrus.FieldLogger, apiNodeLifecycleDiscoveryEnabled bool) (types.Provider, error) {
	var opts []aws.Opt

	if cfg := config.Get().EKS; cfg != nil {
		opts = append(opts, aws.WithMetadata(cfg.AccountID, cfg.Region, cfg.ClusterName))
		opts = append(opts, aws.WithAPITimeout(cfg.APITimeout))
	} else {
		opts = append(opts, aws.WithMetadataDiscovery())
	}

	opts = append(opts, aws.WithEC2Client())

	awsClient, err := aws.New(ctx, log, opts...)
	if err != nil {
		return nil, fmt.Errorf("configuring aws client: %w", err)
	}

	return aws.NewProvider(log, Name, awsClient, apiNodeLifecycleDiscoveryEnabled, newRegisterClusterBuilder(awsClient))
}
