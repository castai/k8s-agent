package aws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2_types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"castai-agent/internal/castai"
	mock_castai "castai-agent/internal/castai/mock"
	"castai-agent/internal/services/providers/eks/aws/mock"
	"castai-agent/internal/services/providers/types"
	"castai-agent/pkg/labels"
)

func TestProvider_RegisterCluster(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	mockctrl := gomock.NewController(t)
	castClient := mock_castai.NewMockClient(mockctrl)
	reqBuilder := mock_aws.NewMockRegisterClusterBuilder(mockctrl)

	p := &Provider{
		log:                    logrus.New(),
		registerClusterBuilder: reqBuilder,
	}

	reqBuilder.EXPECT().BuildRegisterClusterRequest(ctx).Return(&castai.RegisterClusterRequest{
		Name: "test",
		EKS: &castai.EKSParams{
			ClusterName: "test",
			Region:      "eu-central-1",
			AccountID:   "id",
		},
	}, nil)

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
		awsClient := mock_aws.NewMockClient(gomock.NewController(t))

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

	t.Run("spot instance worker label", func(t *testing.T) {
		r := require.New(t)
		awsClient := mock_aws.NewMockClient(gomock.NewController(t))

		p := &Provider{
			log:                              logrus.New(),
			awsClient:                        awsClient,
			apiNodeLifecycleDiscoveryEnabled: true,
			spotCache:                        map[string]bool{},
		}

		node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			labels.WorkerSpot: "true",
		}}}

		got, err := p.FilterSpot(context.Background(), []*v1.Node{node})

		r.NoError(err)
		r.Equal([]*v1.Node{node}, got)
	})

	t.Run("spot instance CAST AI label", func(t *testing.T) {
		r := require.New(t)
		awsClient := mock_aws.NewMockClient(gomock.NewController(t))

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
		awsClient := mock_aws.NewMockClient(gomock.NewController(t))

		p := &Provider{
			log:                              logrus.New(),
			awsClient:                        awsClient,
			apiNodeLifecycleDiscoveryEnabled: true,
			spotCache:                        map[string]bool{},
		}

		awsClient.EXPECT().GetInstancesByInstanceIDs(gomock.Any(), []string{"instanceID"}).Return([]ec2_types.Instance{
			{
				InstanceId:        ptr.To("instanceID"),
				InstanceLifecycle: ec2_types.InstanceLifecycleTypeSpot,
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
		awsClient := mock_aws.NewMockClient(gomock.NewController(t))

		p := &Provider{
			log:                              logrus.New(),
			awsClient:                        awsClient,
			apiNodeLifecycleDiscoveryEnabled: true,
			spotCache:                        map[string]bool{},
		}

		awsClient.EXPECT().GetInstancesByInstanceIDs(gomock.Any(), []string{"instanceID"}).Return([]ec2_types.Instance{
			{
				InstanceId:        ptr.To("instanceID"),
				InstanceLifecycle: ec2_types.InstanceLifecycleTypeScheduled,
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
		awsClient := mock_aws.NewMockClient(gomock.NewController(t))

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
		awsClient := mock_aws.NewMockClient(gomock.NewController(t))

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

func TestClusterNameFromTags(t *testing.T) {
	randomTag1 := ec2_types.Tag{
		Key:   ptr.To("random1"),
		Value: ptr.To("value1"),
	}
	randomTag2 := ec2_types.Tag{
		Key:   ptr.To("random2"),
		Value: ptr.To("value2"),
	}

	tests := []struct {
		name                string
		tags                []ec2_types.Tag
		expectedClusterName string
	}{
		{
			name: "eks tag 1",
			tags: []ec2_types.Tag{
				{
					Key:   ptr.To(tagEKSK8sCluster + "eks-tag-1"),
					Value: ptr.To(owned),
				},
				randomTag1,
				randomTag2,
			},
			expectedClusterName: "eks-tag-1",
		},
		{
			name: "eks tag 2",
			tags: []ec2_types.Tag{
				{
					Key:   ptr.To(tagEKSK8sCluster + "eks-tag-2"),
					Value: ptr.To(owned),
				},
				randomTag1,
				randomTag2,
			},
			expectedClusterName: "eks-tag-2",
		},
		{
			name: "kops tag",
			tags: []ec2_types.Tag{
				{
					Key:   ptr.To(tagKOPSKubernetesCluster),
					Value: ptr.To("kops-tag"),
				},
				randomTag1,
				randomTag2,
			},
			expectedClusterName: "kops-tag",
		},
		{
			name: "all tags",
			tags: []ec2_types.Tag{
				{
					Key:   ptr.To(tagEKSK8sCluster + "all-tags"),
					Value: ptr.To(owned),
				},
				{
					Key:   ptr.To(tagEKSK8sCluster + "all-tags"),
					Value: ptr.To(owned),
				},
				{
					Key:   ptr.To(tagKOPSKubernetesCluster),
					Value: ptr.To("all-tags"),
				},
				randomTag1,
				randomTag2,
			},
			expectedClusterName: "all-tags",
		},
		{
			name: "no tags cluster tags",
			tags: []ec2_types.Tag{
				randomTag1,
				randomTag2,
			},
			expectedClusterName: "",
		},
		{
			name:                "no tags at all",
			tags:                nil,
			expectedClusterName: "",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			got := getClusterName(test.tags)
			require.Equal(t, test.expectedClusterName, got)
		})
	}
}

func Test_APITimeout(t *testing.T) {
	tests := map[string]struct {
		apiTimeout      time.Duration
		requestDuration time.Duration
		expectTimeout   bool
	}{
		"should fail with timeout error when request from API is not received within timeout duration": {
			apiTimeout:      10 * time.Millisecond,
			requestDuration: 50 * time.Millisecond,
			expectTimeout:   true,
		},
		"should not fail when request from API is received within timeout duration": {
			apiTimeout:      200 * time.Millisecond,
			requestDuration: 50 * time.Millisecond,
			expectTimeout:   false,
		},
	}

	for testName, test := range tests {
		test := test

		t.Run(testName, func(t *testing.T) {
			r := require.New(t)
			ctx := context.Background()
			log := logrus.New()

			withMockEC2Client := func(ctx context.Context, c *client) error {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(test.requestDuration)
					w.WriteHeader(http.StatusOK)
				}))

				s := aws.Config{
					Region:       "us-central1",
					Credentials:  aws.AnonymousCredentials{},
					BaseEndpoint: ptr.To(server.URL),
				}
				c.sess = s
				c.ec2Client = ec2.NewFromConfig(s)

				return nil
			}

			client, err := New(ctx, log, withMockEC2Client, WithAPITimeout(test.apiTimeout))
			r.NoError(err)
			r.NotNil(client)

			_, err = client.GetInstancesByInstanceIDs(ctx, []string{"1"})
			if test.expectTimeout {
				r.Error(err)
				r.Contains(err.Error(), "context deadline exceeded")
			} else {
				r.NoError(err)
			}
		})
	}
}
