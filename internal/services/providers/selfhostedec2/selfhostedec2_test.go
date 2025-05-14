package selfhostedec2

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"castai-agent/internal/services/providers/eks/aws/mock"
)

func TestSelfHostedEC2RegisterClusterRequestBuilder(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	mockctrl := gomock.NewController(t)
	awsClient := mock_aws.NewMockClient(mockctrl)

	awsClient.EXPECT().GetClusterName(ctx).Return(lo.ToPtr("test-cluster"), nil)
	awsClient.EXPECT().GetRegion(ctx).Return(lo.ToPtr("eu-central-1"), nil)
	awsClient.EXPECT().GetAccountID(ctx).Return(lo.ToPtr("account-id"), nil)

	builder := newRegisterClusterBuilder(awsClient)
	req, err := builder.BuildRegisterClusterRequest(ctx)
	r.NoError(err)
	r.Equal("test-cluster", req.Name)
	r.Equal("test-cluster", req.SelfHostedWithEC2Nodes.ClusterName)
	r.Equal("eu-central-1", req.SelfHostedWithEC2Nodes.Region)
	r.Equal("account-id", req.SelfHostedWithEC2Nodes.AccountID)
}
