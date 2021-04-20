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
)

func TestProvider_RegisterCluster(t *testing.T) {
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
		EKS: castai.EKSParams{
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

	require.NoError(t, err)
	require.Equal(t, expected, got)
}

func TestProvider_FilterSpot(t *testing.T) {
	t.Run("no spot instances", func(t *testing.T) {
		ctx := context.Background()
		awsClient := mock_client.NewMockClient(gomock.NewController(t))

		p := &Provider{
			log:       logrus.New(),
			awsClient: awsClient,
		}

		nodes := []*v1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						v1.LabelHostname: "hostname",
					},
				},
			},
		}

		instances := []*ec2.Instance{
			{
				PrivateDnsName:    pointer.StringPtr("hostname"),
				InstanceLifecycle: pointer.StringPtr("on-demand"),
			},
		}

		awsClient.EXPECT().GetInstancesByPrivateDNS(ctx, []string{"hostname"}).Return(instances, nil)

		got, err := p.FilterSpot(ctx, nodes)

		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("one spot instance", func(t *testing.T) {
		ctx := context.Background()
		awsClient := mock_client.NewMockClient(gomock.NewController(t))

		p := &Provider{
			log:       logrus.New(),
			awsClient: awsClient,
		}

		spotNode := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "spot",
				Labels: map[string]string{
					v1.LabelHostname: "spot",
				},
			},
		}

		nodes := []*v1.Node{spotNode, {
			ObjectMeta: metav1.ObjectMeta{
				Name: "on-demand",
				Labels: map[string]string{
					v1.LabelHostname: "on-demand",
				},
			},
		}}

		instances := []*ec2.Instance{
			{
				PrivateDnsName:    pointer.StringPtr("spot"),
				InstanceLifecycle: pointer.StringPtr("spot"),
			},
			{
				PrivateDnsName:    pointer.StringPtr("on-demand"),
				InstanceLifecycle: pointer.StringPtr("on-demand"),
			},
		}

		awsClient.EXPECT().GetInstancesByPrivateDNS(ctx, []string{"spot", "on-demand"}).Return(instances, nil)

		got, err := p.FilterSpot(ctx, nodes)

		require.NoError(t, err)
		require.Equal(t, []*v1.Node{spotNode}, got)
	})

	t.Run("should use cache", func(t *testing.T) {
		ctx := context.Background()
		awsClient := mock_client.NewMockClient(gomock.NewController(t))

		p := &Provider{
			log:       logrus.New(),
			awsClient: awsClient,
		}

		nodes := []*v1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						v1.LabelHostname: "hostname",
					},
				},
			},
		}

		instances := []*ec2.Instance{
			{
				PrivateDnsName:    pointer.StringPtr("hostname"),
				InstanceLifecycle: pointer.StringPtr("on-demand"),
			},
		}

		awsClient.EXPECT().GetInstancesByPrivateDNS(ctx, []string{"hostname"}).Times(1).Return(instances, nil)

		got, err := p.FilterSpot(ctx, nodes)

		require.NoError(t, err)
		require.Empty(t, got)

		got, err = p.FilterSpot(ctx, nodes)

		require.NoError(t, err)
		require.Empty(t, got)
	})
}
