package selfhostedec2

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	aws_mocks "castai-agent/mocks/internal_/services/providers/eks/aws"
)

func TestSelfHostedEC2RegisterClusterRequestBuilder(t *testing.T) {
	r := require.New(t)
	ctx := t.Context()

	awsClient := aws_mocks.NewMockClient(t)
	awsClient.EXPECT().GetClusterName(mock.Anything).Return(lo.ToPtr("test-cluster"), nil)
	awsClient.EXPECT().GetRegion(mock.Anything).Return(lo.ToPtr("eu-central-1"), nil)
	awsClient.EXPECT().GetAccountID(mock.Anything).Return(lo.ToPtr("account-id"), nil)

	builder := newRegisterClusterBuilder(awsClient)
	req, err := builder.BuildRegisterClusterRequest(ctx)
	r.NoError(err)
	r.Equal("test-cluster", req.Name)
	r.Equal("test-cluster", req.SelfHostedWithEC2Nodes.ClusterName)
	r.Equal("eu-central-1", req.SelfHostedWithEC2Nodes.Region)
	r.Equal("account-id", req.SelfHostedWithEC2Nodes.AccountID)
}
