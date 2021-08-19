package gke

import (
	"context"
	"os"
	"testing"

	mock_client "castai-agent/internal/services/providers/gke/client/mock"

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
	ctrl := gomock.NewController(t)
	castaiclient := mock_castai.NewMockClient(ctrl)
	metaclient := mock_client.NewMockMetadata(ctrl)

	p := &Provider{log: logrus.New(), metadata: metaclient}

	require.NoError(t, os.Setenv("API_KEY", "abc"))
	require.NoError(t, os.Setenv("API_URL", "example.com"))

	metaclient.EXPECT().GetClusterName().Return("test-cluster", nil)
	metaclient.EXPECT().GetRegion().Return("us-east4", nil)
	metaclient.EXPECT().GetProjectID().Return("test-project", nil)
	metaclient.EXPECT().GetLocation().Return("us-east4-a", nil)

	resp := &castai.RegisterClusterResponse{Cluster: castai.Cluster{
		ID:             uuid.New().String(),
		OrganizationID: uuid.New().String(),
	}}
	castaiclient.EXPECT().RegisterCluster(gomock.Any(), &castai.RegisterClusterRequest{
		Name: "test-cluster",
		GKE: &castai.GKEParams{
			Region:      "us-east4",
			ProjectID:   "test-project",
			ClusterName: "test-cluster",
			Location:    "us-east4-a",
		},
	}).Return(resp, nil)

	got, err := p.RegisterCluster(context.Background(), castaiclient)

	require.NoError(t, err)
	require.Equal(t, &types.ClusterRegistration{
		ClusterID:      resp.ID,
		OrganizationID: resp.OrganizationID,
	}, got)
}

func TestProvider_IsSpot(t *testing.T) {
	p := &Provider{log: logrus.New()}

	tests := []struct {
		name     string
		node     *v1.Node
		expected bool
	}{
		{
			name:     "castai spot node",
			node:     &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{labels.CastaiSpot: "true"}}},
			expected: true,
		},
		{
			name:     "gke spot node",
			node:     &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{LabelPreemptible: "true"}}},
			expected: true,
		},
		{
			name:     "on demand node",
			node:     &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{}}},
			expected: false,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			got, err := p.IsSpot(context.Background(), test.node)
			require.NoError(t, err)
			require.Equal(t, test.expected, got)
		})
	}
}
