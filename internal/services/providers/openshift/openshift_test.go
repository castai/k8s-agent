package openshift

import (
	"context"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"castai-agent/internal/castai"
	mock_castai "castai-agent/internal/castai/mock"
	"castai-agent/internal/config"
	mock_discovery "castai-agent/internal/services/discovery/mock"
	"castai-agent/internal/services/providers/types"
)

func TestProvider_RegisterCluster(t *testing.T) {
	regReq := &castai.RegisterClusterRequest{
		ID:   uuid.New(),
		Name: "test-cluster",
		Openshift: &castai.OpenshiftParams{
			CSP:         "aws",
			Region:      "us-east-1",
			ClusterName: "test-cluster",
			InternalID:  uuid.New().String(),
		},
	}

	tests := []struct {
		name                 string
		config               func(t *testing.T)
		discoveryServiceMock func(t *testing.T, mock *mock_discovery.MockService)
	}{
		{
			name: "should autodiscover cluster properties from kubernetes state using the discovery service",
			config: func(t *testing.T) {
				r := require.New(t)
				r.NoError(os.Setenv("API_KEY", "123"))
				r.NoError(os.Setenv("API_URL", "test"))
			},
			discoveryServiceMock: func(t *testing.T, discoveryService *mock_discovery.MockService) {
				discoveryService.EXPECT().GetClusterID(gomock.Any()).Return(&regReq.ID, nil)
				discoveryService.EXPECT().GetCSPAndRegion(gomock.Any()).Return(&regReq.Openshift.CSP, &regReq.Openshift.Region, nil)
				discoveryService.EXPECT().GetOpenshiftClusterID(gomock.Any()).Return(&regReq.Openshift.InternalID, nil)
				discoveryService.EXPECT().GetOpenshiftClusterName(gomock.Any()).Return(&regReq.Name, nil)
			},
		},
		{
			name: "should use config values if provided",
			config: func(t *testing.T) {
				r := require.New(t)
				r.NoError(os.Setenv("API_KEY", "123"))
				r.NoError(os.Setenv("API_URL", "test"))
				r.NoError(os.Setenv("OPENSHIFT_CSP", regReq.Openshift.CSP))
				r.NoError(os.Setenv("OPENSHIFT_REGION", regReq.Openshift.Region))
				r.NoError(os.Setenv("OPENSHIFT_CLUSTER_NAME", regReq.Name))
				r.NoError(os.Setenv("OPENSHIFT_INTERNAL_ID", regReq.Openshift.InternalID))
			},
			discoveryServiceMock: func(t *testing.T, discoveryService *mock_discovery.MockService) {
				discoveryService.EXPECT().GetClusterID(gomock.Any()).Return(&regReq.ID, nil)
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			r := require.New(t)

			if test.config != nil {
				test.config(t)
				t.Cleanup(config.Reset)
				t.Cleanup(os.Clearenv)
			}

			mockctrl := gomock.NewController(t)
			castaiclient := mock_castai.NewMockClient(mockctrl)
			discoveryService := mock_discovery.NewMockService(mockctrl)

			p := New(discoveryService, nil)

			if test.discoveryServiceMock != nil {
				test.discoveryServiceMock(t, discoveryService)
			}

			expected := &types.ClusterRegistration{
				ClusterID:      regReq.ID.String(),
				OrganizationID: uuid.New().String(),
			}

			castaiclient.EXPECT().RegisterCluster(gomock.Any(), regReq).
				Return(&castai.RegisterClusterResponse{
					Cluster: castai.Cluster{
						ID:             regReq.ID.String(),
						OrganizationID: expected.OrganizationID,
					},
				}, nil)

			got, err := p.RegisterCluster(context.Background(), castaiclient)

			r.NoError(err)
			r.Equal(expected, got)
		})
	}
}
