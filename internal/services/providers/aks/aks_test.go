package aks

import (
	"context"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"castai-agent/internal/castai"
	mock_castai "castai-agent/internal/castai/mock"
	"castai-agent/internal/services/providers/types"
	"castai-agent/pkg/labels"
)

func TestProvider_RegisterCluster(t *testing.T) {
	t.Run("happy_path", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		castaiclient := mock_castai.NewMockClient(ctrl)

		p := &Provider{log: logrus.New()}

		require.NoError(t, os.Setenv("API_KEY", "abc"))
		require.NoError(t, os.Setenv("API_URL", "example.com"))

		require.NoError(t, os.Setenv("AKS_SUBSCRIPTION_ID", "test-id"))
		require.NoError(t, os.Setenv("AKS_LOCATION", "test-location"))
		require.NoError(t, os.Setenv("AKS_NODE_RESOURCE_GROUP", "test-group"))

		resp := &castai.RegisterClusterResponse{Cluster: castai.Cluster{
			ID:             uuid.New().String(),
			OrganizationID: uuid.New().String(),
		}}

		castaiclient.EXPECT().RegisterCluster(gomock.Any(), &castai.RegisterClusterRequest{
			AKS: &castai.AKSParams{
				Region:            "test-location",
				SubscriptionID:    "test-id",
				NodeResourceGroup: "test-group",
			},
		}).Return(resp, nil)

		got, err := p.RegisterCluster(context.Background(), castaiclient)

		require.NoError(t, err)
		require.Equal(t, &types.ClusterRegistration{
			ClusterID:      resp.ID,
			OrganizationID: resp.OrganizationID,
		}, got)
	})
}

func TestProvider_IsSpot(t *testing.T) {
	t.Run("spot instance priority label", func(t *testing.T) {
		p := &Provider{
			log: logrus.New(),
		}

		node := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{SpotLabelKey: SpotLabelVal}},
		}

		got, err := p.FilterSpot(context.Background(), []*v1.Node{node})

		require.NoError(t, err)
		require.Equal(t, []*v1.Node{node}, got)
	})

	t.Run("spot instance karpenter label", func(t *testing.T) {
		p := &Provider{
			log: logrus.New(),
		}

		node := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{labels.KarpenterCapacityType: labels.ValueKarpenterCapacityTypeSpot}},
		}

		got, err := p.FilterSpot(context.Background(), []*v1.Node{node})

		require.NoError(t, err)
		require.Equal(t, []*v1.Node{node}, got)
	})
}
