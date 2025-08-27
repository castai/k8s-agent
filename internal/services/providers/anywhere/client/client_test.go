package client

import (
	"context"
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"castai-agent/internal/services/discovery"
	discovery_mock "castai-agent/mocks/internal_/services/discovery"
	eks_client_mock "castai-agent/mocks/internal_/services/providers/eks/aws"
	gke_client_mock "castai-agent/mocks/internal_/services/providers/gke/client"
	"castai-agent/pkg/cloud"
)

func Test_GetClusterName(t *testing.T) {

	tests := map[string]struct {
		csp                 cloud.Cloud
		eksClusterName      *string
		gkeClusterName      string
		expectedClusterName string
		expectedErr         *string
		getClient           func(context.Context, testing.TB, logrus.FieldLogger, discovery.Service) *client
	}{
		"should fail to determine cluster name when CSP is not determined": {
			getClient: func(_ context.Context, _ testing.TB, log logrus.FieldLogger, discoveryService discovery.Service) *client {
				return &client{
					log:              logrus.New(),
					discoveryService: discoveryService,
				}
			},
			expectedErr: lo.ToPtr("cluster name could not be determined automatically"),
		},
		"should use EKS client to determine cluster name when CSP is AWS": {
			csp: cloud.AWS,
			getClient: func(ctx context.Context, t testing.TB, log logrus.FieldLogger, discoveryService discovery.Service) *client {
				eksClient := eks_client_mock.NewMockClient(t)
				eksClient.EXPECT().GetClusterName(ctx).Return(lo.ToPtr("eks-cluster-name"), nil).Times(1)

				return &client{
					log:              logrus.New(),
					discoveryService: discoveryService,
					eksClient:        eksClient,
				}
			},
			expectedClusterName: "eks-cluster-name",
		},
		"should return EKS client's error when CSP is AWS and underlying client returns an error": {
			csp: cloud.AWS,
			getClient: func(ctx context.Context, t testing.TB, log logrus.FieldLogger, discoveryService discovery.Service) *client {
				eksClient := eks_client_mock.NewMockClient(t)
				eksClient.EXPECT().GetClusterName(ctx).Return(nil, fmt.Errorf("eks error")).Times(1)

				return &client{
					log:              logrus.New(),
					discoveryService: discoveryService,
					eksClient:        eksClient,
				}
			},
			expectedErr: lo.ToPtr("eks error"),
		},
		"should use GKE client to determine cluster name when CSP is GCP": {
			csp: cloud.GCP,
			getClient: func(ctx context.Context, t testing.TB, log logrus.FieldLogger, discoveryService discovery.Service) *client {
				gkeClient := gke_client_mock.NewMockMetadata(t)
				gkeClient.EXPECT().GetClusterName().Return("gke-cluster-name", nil).Times(1)

				return &client{
					log:               logrus.New(),
					discoveryService:  discoveryService,
					gkeMetadataClient: gkeClient,
				}
			},
			expectedClusterName: "gke-cluster-name",
		},
		"should return GKE client's error when CSP is GCP and underlying client returns an error": {
			csp: cloud.GCP,
			getClient: func(ctx context.Context, t testing.TB, log logrus.FieldLogger, discoveryService discovery.Service) *client {
				gkeClient := gke_client_mock.NewMockMetadata(t)
				gkeClient.EXPECT().GetClusterName().Return("", fmt.Errorf("gke error")).Times(1)

				return &client{
					log:               logrus.New(),
					discoveryService:  discoveryService,
					gkeMetadataClient: gkeClient,
				}
			},
			expectedErr: lo.ToPtr("gke error"),
		},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			r := require.New(t)

			discoveryService := discovery_mock.NewMockService(t)

			discoveryService.EXPECT().GetCSP(ctx).Return(test.csp, nil).Times(1)

			client := test.getClient(ctx, t, logrus.New(), discoveryService)

			clusterName, err := client.GetClusterName(ctx)

			if test.expectedErr != nil {
				r.ErrorContains(err, *test.expectedErr)
			} else {
				r.Equal(clusterName, test.expectedClusterName)
			}
		})
	}
}
