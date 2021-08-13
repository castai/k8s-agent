package gke

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
	castaiclient := mock_castai.NewMockClient(gomock.NewController(t))

	p := &Provider{log: logrus.New()}

	require.NoError(t, os.Setenv("API_KEY", "abc"))
	require.NoError(t, os.Setenv("API_URL", "example.com"))
	require.NoError(t, os.Setenv("GKE_REGION", "us-east4"))
	require.NoError(t, os.Setenv("GKE_PROJECT_ID", "test-abc"))
	require.NoError(t, os.Setenv("GKE_CLUSTER_NAME", "test-cluster"))

	resp := &castai.RegisterClusterResponse{Cluster: castai.Cluster{
		ID:             uuid.New().String(),
		OrganizationID: uuid.New().String(),
	}}
	castaiclient.EXPECT().RegisterCluster(gomock.Any(), &castai.RegisterClusterRequest{
		Name: "test-cluster",
		GKE: &castai.GKEParams{
			Region:      "us-east4",
			ProjectID:   "test-abc",
			ClusterName: "test-cluster",
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
