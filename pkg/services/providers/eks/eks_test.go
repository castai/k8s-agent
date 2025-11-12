package eks

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	mock_aws "castai-agent/mocks/pkg_/services/providers/eks/aws"
)

func TestEKSRegisterClusterRequestBuilder(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	awsClient := mock_aws.NewMockClient(t)

	awsClient.EXPECT().GetClusterName(ctx).Return(lo.ToPtr("test-cluster"), nil)
	awsClient.EXPECT().GetRegion(ctx).Return(lo.ToPtr("eu-central-1"), nil)
	awsClient.EXPECT().GetAccountID(ctx).Return(lo.ToPtr("account-id"), nil)

	builder := newRegisterClusterBuilder(awsClient)
	req, err := builder.BuildRegisterClusterRequest(ctx)
	r.NoError(err)
	r.Equal("test-cluster", req.Name)
	r.Equal("test-cluster", req.EKS.ClusterName)
	r.Equal("eu-central-1", req.EKS.Region)
	r.Equal("account-id", req.EKS.AccountID)
}
