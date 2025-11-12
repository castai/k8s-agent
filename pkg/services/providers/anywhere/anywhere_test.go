package anywhere

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"castai-agent/pkg/castai"
	"castai-agent/internal/config"
	mock_castai "castai-agent/mocks/pkg_/castai"
	discovery_mock "castai-agent/mocks/internal_/services/discovery"
	anywhere_client_mock "castai-agent/mocks/pkg_/services/providers/anywhere/client"
)

func Test_RegisterCluster(t *testing.T) {
	clusterID := uuid.New()
	orgID := uuid.New()
	kubeNsId := uuid.New()

	tests := map[string]struct {
		expectedClusterName     string
		expectedKubeNamespaceId uuid.UUID
		expectedErr             *string
		setup                   func(testing.TB, *require.Assertions) (*discovery_mock.MockService, *anywhere_client_mock.MockClient)
	}{
		"should fail cluster registration when there's an error retrieving kube-system namespace id": {
			setup: func(t testing.TB, _ *require.Assertions) (*discovery_mock.MockService, *anywhere_client_mock.MockClient) {
				discoveryService := discovery_mock.NewMockService(t)
				anywhereClient := anywhere_client_mock.NewMockClient(t)

				discoveryService.EXPECT().GetKubeSystemNamespaceID(mock.Anything).Return(nil, errors.New("some error"))

				return discoveryService, anywhereClient
			},
			expectedErr: lo.ToPtr("getting kube-system namespace id"),
		},
		"should use cluster name from config when it's provided through env variables": {
			setup: func(t testing.TB, r *require.Assertions) (*discovery_mock.MockService, *anywhere_client_mock.MockClient) {
				r.NoError(os.Setenv("ANYWHERE_CLUSTER_NAME", "env-cluster-name"))

				discoveryService := discovery_mock.NewMockService(t)
				anywhereClient := anywhere_client_mock.NewMockClient(t)

				discoveryService.EXPECT().GetKubeSystemNamespaceID(mock.Anything).Return(&kubeNsId, nil)

				return discoveryService, anywhereClient
			},
			expectedClusterName:     "env-cluster-name",
			expectedKubeNamespaceId: kubeNsId,
		},
		"should use cluster name from client when it's provided through env variables": {
			setup: func(t testing.TB, _ *require.Assertions) (*discovery_mock.MockService, *anywhere_client_mock.MockClient) {
				discoveryService := discovery_mock.NewMockService(t)
				anywhereClient := anywhere_client_mock.NewMockClient(t)

				discoveryService.EXPECT().GetKubeSystemNamespaceID(mock.Anything).Return(&kubeNsId, nil)
				anywhereClient.EXPECT().GetClusterName(mock.Anything).Return("client-cluster-name", nil)

				return discoveryService, anywhereClient
			},
			expectedClusterName:     "client-cluster-name",
			expectedKubeNamespaceId: kubeNsId,
		},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Cleanup(config.Reset)
			t.Cleanup(os.Clearenv)

			ctx := context.Background()
			r := require.New(t)

			discoveryService, anywhereClient := test.setup(t, r)
			castaiClient := mock_castai.NewMockClient(t)

			provider := New(discoveryService, anywhereClient, logrus.New())

			if test.expectedErr == nil {
				castaiClient.EXPECT().RegisterCluster(mock.Anything, &castai.RegisterClusterRequest{
					Name: test.expectedClusterName,
					Anywhere: &castai.AnywhereParams{
						ClusterName:           test.expectedClusterName,
						KubeSystemNamespaceID: test.expectedKubeNamespaceId,
					},
				}).Return(&castai.RegisterClusterResponse{
					Cluster: castai.Cluster{
						ID:             clusterID.String(),
						OrganizationID: orgID.String(),
					},
				}, nil).Times(1)
			}

			result, err := provider.RegisterCluster(ctx, castaiClient)

			if test.expectedErr != nil {
				r.ErrorContains(err, *test.expectedErr)
			} else {
				r.Equal(clusterID.String(), result.ClusterID)
				r.Equal(orgID.String(), result.OrganizationID)
			}
		})
	}
}
