package selfhostedec2

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/golang/mock/gomock"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"castai-agent/internal/castai"
	mock_castai "castai-agent/internal/castai/mock"
	mock_aws "castai-agent/internal/services/providers/selfhostedec2/mock"
	"castai-agent/pkg/labels"
)

func TestProvider_RegisterCluster(t *testing.T) {
	tests := []struct {
		name              string
		configureProvider func(*Provider)
		configureIMDS     func(*mock_aws.MockIMDSClient)
		configureCastAI   func(*mock_castai.MockClient)
		castaiResp        *castai.RegisterClusterResponse
		wantErr           bool
		expectedReq       *castai.RegisterClusterRequest
	}{
		{
			name: "successful registration with config values",
			configureProvider: func(p *Provider) {
				p.clusterName = "test-cluster"
				p.region = "us-west-2"
				p.accountID = "123456789012"
			},
			configureIMDS: nil,
			castaiResp: &castai.RegisterClusterResponse{
				Cluster: castai.Cluster{
					ID:             "cluster-id",
					OrganizationID: "org-id",
				},
			},
			configureCastAI: func(m *mock_castai.MockClient) {
				m.EXPECT().
					RegisterCluster(gomock.Any(), gomock.Any()).
					Return(&castai.RegisterClusterResponse{
						Cluster: castai.Cluster{
							ID:             "cluster-id",
							OrganizationID: "org-id",
						},
					}, nil)
			},
			expectedReq: &castai.RegisterClusterRequest{
				Name: "test-cluster",
				SelfHostedWithEC2Nodes: &castai.SelfHostedWithEC2NodesParams{
					ClusterName: "test-cluster",
					Region:      "us-west-2",
					AccountID:   "123456789012",
				},
			},
		},
		{
			name: "successful registration with IMDS values",
			configureIMDS: func(m *mock_aws.MockIMDSClient) {
				m.EXPECT().
					GetInstanceIdentityDocument(gomock.Any(), gomock.Any()).
					Return(&imds.GetInstanceIdentityDocumentOutput{
						InstanceIdentityDocument: imds.InstanceIdentityDocument{
							Region:    "us-east-1",
							AccountID: "987654321098",
						},
					}, nil)
				m.EXPECT().
					GetMetadata(gomock.Any(), &imds.GetMetadataInput{
						Path: "tags/instance/castai:cluster-name",
					}).
					Return(&imds.GetMetadataOutput{
						Content: io.NopCloser(strings.NewReader("test-cluster-imds")),
					}, nil)
			},
			configureCastAI: func(m *mock_castai.MockClient) {
				m.EXPECT().
					RegisterCluster(gomock.Any(), gomock.Any()).
					Return(&castai.RegisterClusterResponse{
						Cluster: castai.Cluster{
							ID:             "cluster-id",
							OrganizationID: "org-id",
						},
					}, nil)
			},
			castaiResp: &castai.RegisterClusterResponse{
				Cluster: castai.Cluster{
					ID:             "cluster-id",
					OrganizationID: "org-id",
				},
			},
			expectedReq: &castai.RegisterClusterRequest{
				Name: "test-cluster-imds",
				SelfHostedWithEC2Nodes: &castai.SelfHostedWithEC2NodesParams{
					ClusterName: "test-cluster-imds",
					Region:      "us-east-1",
					AccountID:   "987654321098",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			imdsClient := mock_aws.NewMockIMDSClient(ctrl)
			castaiClient := mock_castai.NewMockClient(ctrl)

			if tt.configureIMDS != nil {
				tt.configureIMDS(imdsClient)
			}
			if tt.configureCastAI != nil {
				tt.configureCastAI(castaiClient)
			}

			p := &Provider{
				log:  logrus.New(),
				imds: imdsClient,
			}
			if tt.configureProvider != nil {
				tt.configureProvider(p)
			}

			result, err := p.RegisterCluster(context.Background(), castaiClient)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.castaiResp.Cluster.ID, result.ClusterID)
			assert.Equal(t, tt.castaiResp.Cluster.OrganizationID, result.OrganizationID)
		})
	}
}

func TestProvider_FilterSpot(t *testing.T) {
	tests := []struct {
		name         string
		nodes        []*v1.Node
		setupMocks   func(*mock_aws.MockEC2Client)
		expectedSpot []*v1.Node
		apiDiscovery bool
		wantErr      bool
	}{
		{
			name: "filter nodes with castai spot label",
			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							labels.CastaiSpot: "true",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							labels.CastaiSpot: "false",
						},
					},
				},
			},
			expectedSpot: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							labels.CastaiSpot: "true",
						},
					},
				},
			},
		},
		{
			name: "filter nodes with karpenter capacity type",
			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							labels.KarpenterCapacityType: labels.ValueKarpenterCapacityTypeSpot,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							labels.KarpenterCapacityType: labels.ValueKarpenterCapacityTypeOnDemand,
						},
					},
				},
			},
			expectedSpot: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							labels.KarpenterCapacityType: labels.ValueKarpenterCapacityTypeSpot,
						},
					},
				},
			},
		},
		{
			name:         "filter nodes with API discovery",
			apiDiscovery: true,
			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
					Spec: v1.NodeSpec{
						ProviderID: "aws:///us-west-2a/i-1234567890abcdef0",
					},
				},
			},
			setupMocks: func(ec2Client *mock_aws.MockEC2Client) {
				ec2Client.EXPECT().
					DescribeInstances(gomock.Any(), &ec2.DescribeInstancesInput{
						InstanceIds: []string{"i-1234567890abcdef0"},
					}).
					Return(&ec2.DescribeInstancesOutput{
						Reservations: []ec2types.Reservation{
							{
								Instances: []ec2types.Instance{
									{
										InstanceId:        aws.String("i-1234567890abcdef0"),
										InstanceLifecycle: ec2types.InstanceLifecycleTypeSpot,
									},
								},
							},
						},
					}, nil)
			},
			expectedSpot: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
					Spec: v1.NodeSpec{
						ProviderID: "aws:///us-west-2a/i-1234567890abcdef0",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Client := mock_aws.NewMockEC2Client(ctrl)
			if tt.setupMocks != nil {
				tt.setupMocks(ec2Client)
			}

			p := &Provider{
				log:                              logrus.New(),
				ec2:                              ec2Client,
				spotCache:                        make(map[string]bool),
				apiNodeLifecycleDiscoveryEnabled: tt.apiDiscovery,
			}

			result, err := p.FilterSpot(context.Background(), tt.nodes)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, len(tt.expectedSpot), len(result))
			for i := range tt.expectedSpot {
				assert.Equal(t, tt.expectedSpot[i].Name, result[i].Name)
			}
		})
	}
}
