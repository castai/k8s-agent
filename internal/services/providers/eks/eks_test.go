package eks

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"castai-agent/internal/castai"
	mock_castai "castai-agent/internal/castai/mock"
	mock_client "castai-agent/internal/services/providers/eks/client/mock"
	"castai-agent/internal/services/providers/types"
	"castai-agent/pkg/labels"
)

func TestProvider_RegisterCluster(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	mockctrl := gomock.NewController(t)
	awsClient := mock_client.NewMockClient(mockctrl)
	castClient := mock_castai.NewMockClient(mockctrl)

	p := &Provider{
		log:       logrus.New(),
		awsClient: awsClient,
	}

	awsClient.EXPECT().GetClusterName(ctx).Return(pointer.StringPtr("test"), nil)
	awsClient.EXPECT().GetRegion(ctx).Return(pointer.StringPtr("eu-central-1"), nil)
	awsClient.EXPECT().GetAccountID(ctx).Return(pointer.StringPtr("id"), nil)

	expectedReq := &castai.RegisterClusterRequest{
		Name: "test",
		EKS: &castai.EKSParams{
			ClusterName: "test",
			Region:      "eu-central-1",
			AccountID:   "id",
		},
	}

	expected := &types.ClusterRegistration{
		ClusterID:      uuid.New().String(),
		OrganizationID: uuid.New().String(),
	}

	castClient.EXPECT().RegisterCluster(ctx, expectedReq).Return(&castai.RegisterClusterResponse{Cluster: castai.Cluster{
		ID:             expected.ClusterID,
		OrganizationID: expected.OrganizationID,
	}}, nil)

	got, err := p.RegisterCluster(ctx, castClient)

	r.NoError(err)
	r.Equal(expected, got)
}

func TestProvider_IsSpot(t *testing.T) {
	t.Run("spot instance capacity label", func(t *testing.T) {
		r := require.New(t)
		awsClient := mock_client.NewMockClient(gomock.NewController(t))

		p := &Provider{
			log:                              logrus.New(),
			awsClient:                        awsClient,
			apiNodeLifecycleDiscoveryEnabled: true,
			spotCache:                        map[string]bool{},
		}

		node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			LabelCapacity: ValueCapacitySpot,
		}}}

		got, err := p.FilterSpot(context.Background(), []*v1.Node{node})

		r.NoError(err)
		r.Equal([]*v1.Node{node}, got)
	})

	t.Run("spot instance CAST AI label", func(t *testing.T) {
		r := require.New(t)
		awsClient := mock_client.NewMockClient(gomock.NewController(t))

		p := &Provider{
			log:                              logrus.New(),
			awsClient:                        awsClient,
			apiNodeLifecycleDiscoveryEnabled: true,
			spotCache:                        map[string]bool{},
		}

		node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			labels.CastaiSpot: "true",
		}}}

		got, err := p.FilterSpot(context.Background(), []*v1.Node{node})

		r.NoError(err)
		r.Equal([]*v1.Node{node}, got)
	})

	t.Run("spot instance lifecycle response", func(t *testing.T) {
		r := require.New(t)
		awsClient := mock_client.NewMockClient(gomock.NewController(t))

		p := &Provider{
			log:                              logrus.New(),
			awsClient:                        awsClient,
			apiNodeLifecycleDiscoveryEnabled: true,
			spotCache:                        map[string]bool{},
		}

		awsClient.EXPECT().GetInstancesByInstanceIDs(gomock.Any(), []string{"instanceID"}).Return([]*ec2.Instance{
			{
				InstanceId:        pointer.StringPtr("instanceID"),
				InstanceLifecycle: pointer.StringPtr("spot"),
			},
		}, nil).Times(1)

		node := &v1.Node{
			Spec: v1.NodeSpec{
				ProviderID: "aws:///eu-west-1a/instanceID",
			},
		}

		got, err := p.FilterSpot(context.Background(), []*v1.Node{node})

		r.NoError(err)
		r.Equal([]*v1.Node{node}, got)

		got, err = p.FilterSpot(context.Background(), []*v1.Node{node})

		r.NoError(err)
		r.Equal([]*v1.Node{node}, got)
	})

	t.Run("on-demand instance", func(t *testing.T) {
		r := require.New(t)
		awsClient := mock_client.NewMockClient(gomock.NewController(t))

		p := &Provider{
			log:                              logrus.New(),
			awsClient:                        awsClient,
			apiNodeLifecycleDiscoveryEnabled: true,
			spotCache:                        map[string]bool{},
		}

		awsClient.EXPECT().GetInstancesByInstanceIDs(gomock.Any(), []string{"instanceID"}).Return([]*ec2.Instance{
			{
				InstanceId:        pointer.StringPtr("instanceID"),
				InstanceLifecycle: pointer.StringPtr("on-demand"),
			},
		}, nil)

		node := &v1.Node{
			Spec: v1.NodeSpec{
				ProviderID: "aws:///eu-west-1a/instanceID",
			},
		}

		got, err := p.FilterSpot(context.Background(), []*v1.Node{node})

		r.NoError(err)
		r.Empty(got)
	})

	t.Run("should not perform call out to AWS API if node types can be determined using labels", func(t *testing.T) {
		r := require.New(t)
		awsClient := mock_client.NewMockClient(gomock.NewController(t))

		p := &Provider{
			log:                              logrus.New(),
			awsClient:                        awsClient,
			apiNodeLifecycleDiscoveryEnabled: true,
			spotCache:                        map[string]bool{},
		}

		nodeCastaiSpot := &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			labels.CastaiSpot: "true",
		}}}
		nodeCastaiSpotFallback := &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			labels.CastaiSpotFallback: "true",
		}}}

		nodeKarpenterSpot := &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			labels.KarpenterCapacityType: labels.ValueKarpenterCapacityTypeSpot,
		}}}
		nodeKarpenterOnDemand := &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			labels.KarpenterCapacityType: labels.ValueKarpenterCapacityTypeOnDemand,
		}}}

		nodeEKSSpot := &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			LabelCapacity: ValueCapacitySpot,
		}}}
		nodeEKSOnDemand := &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			LabelCapacity: ValueCapacityOnDemand,
		}}}

		got, err := p.FilterSpot(context.Background(), []*v1.Node{nodeCastaiSpot, nodeCastaiSpotFallback, nodeKarpenterSpot, nodeKarpenterOnDemand, nodeEKSSpot, nodeEKSOnDemand})

		r.NoError(err)
		r.Equal([]*v1.Node{nodeCastaiSpot, nodeKarpenterSpot, nodeEKSSpot}, got)
	})

	t.Run("should consider on-demand node lifecycle when node lifecycle could not be discovered using labels and API lifecycle discovery is disabled", func(t *testing.T) {
		r := require.New(t)
		awsClient := mock_client.NewMockClient(gomock.NewController(t))

		p := &Provider{
			log:                              logrus.New(),
			awsClient:                        awsClient,
			apiNodeLifecycleDiscoveryEnabled: false,
			spotCache:                        map[string]bool{},
		}

		awsClient.EXPECT().GetInstancesByInstanceIDs(gomock.Any(), gomock.Any()).Times(0)

		node := &v1.Node{
			Spec: v1.NodeSpec{
				ProviderID: "aws:///eu-west-1a/instanceID",
			},
		}

		got, err := p.FilterSpot(context.Background(), []*v1.Node{node})

		r.NoError(err)
		r.Empty(got)
	})
}
