package anywhere

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"castai-agent/internal/castai"
	mock_castai "castai-agent/internal/castai/mock"
	"castai-agent/internal/config"
	discovery_mock "castai-agent/internal/services/discovery/mock"
	anywhere_client_mock "castai-agent/internal/services/providers/anywhere/client/mock"
)

func Test_RegisterCluster(t *testing.T) {
	clusterID := uuid.New()
	orgID := uuid.New()
	kubeNsId := uuid.New()

	tests := map[string]struct {
		expectedClusterName     string
		expectedKubeNamespaceId uuid.UUID
		expectedErr             *string
		setup                   func(*require.Assertions, *gomock.Controller) (*discovery_mock.MockService, *anywhere_client_mock.MockClient)
	}{
		"should fail cluster registration when there's an error retrieving kube-system namespace id": {
			setup: func(_ *require.Assertions, ctrl *gomock.Controller) (*discovery_mock.MockService, *anywhere_client_mock.MockClient) {
				discoveryService := discovery_mock.NewMockService(ctrl)
				anywhereClient := anywhere_client_mock.NewMockClient(ctrl)

				discoveryService.EXPECT().GetKubeSystemNamespaceID(gomock.Any()).Return(nil, fmt.Errorf("some error")).Times(1)

				return discoveryService, anywhereClient
			},
			expectedErr: lo.ToPtr("getting kube-system namespace id"),
		},
		"should use cluster name from config when it's provided through env variables": {
			setup: func(r *require.Assertions, ctrl *gomock.Controller) (*discovery_mock.MockService, *anywhere_client_mock.MockClient) {
				r.NoError(os.Setenv("ANYWHERE_CLUSTER_NAME", "env-cluster-name"))

				discoveryService := discovery_mock.NewMockService(ctrl)
				anywhereClient := anywhere_client_mock.NewMockClient(ctrl)

				discoveryService.EXPECT().GetKubeSystemNamespaceID(gomock.Any()).Return(&kubeNsId, nil).Times(1)
				anywhereClient.EXPECT().GetClusterName(gomock.Any()).Times(0)

				return discoveryService, anywhereClient
			},
			expectedClusterName:     "env-cluster-name",
			expectedKubeNamespaceId: kubeNsId,
		},
		"should use cluster name from client when it's provided through env variables": {
			setup: func(_ *require.Assertions, ctrl *gomock.Controller) (*discovery_mock.MockService, *anywhere_client_mock.MockClient) {
				discoveryService := discovery_mock.NewMockService(ctrl)
				anywhereClient := anywhere_client_mock.NewMockClient(ctrl)

				discoveryService.EXPECT().GetKubeSystemNamespaceID(gomock.Any()).Return(&kubeNsId, nil).Times(1)
				anywhereClient.EXPECT().GetClusterName(gomock.Any()).Return("client-cluster-name", nil).Times(1)

				return discoveryService, anywhereClient
			},
			expectedClusterName:     "client-cluster-name",
			expectedKubeNamespaceId: kubeNsId,
		},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			t.Cleanup(config.Reset)
			t.Cleanup(os.Clearenv)

			ctx := context.Background()
			r := require.New(t)

			discoveryService, anywhereClient := test.setup(r, ctrl)
			castaiClient := mock_castai.NewMockClient(ctrl)

			provider := New(discoveryService, anywhereClient, logrus.New())

			if test.expectedErr == nil {
				castaiClient.EXPECT().RegisterCluster(gomock.Any(), &castai.RegisterClusterRequest{
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
