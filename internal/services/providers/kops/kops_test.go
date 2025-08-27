package kops

import (
	"context"
	"os"
	"strconv"
	"testing"

	ec2_types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/discovery"
	"castai-agent/internal/services/providers/gke"
	"castai-agent/internal/services/providers/types"
	mock_castai "castai-agent/mocks/internal_/castai"
	mock_aws "castai-agent/mocks/internal_/services/providers/eks/aws"
	"castai-agent/pkg/cloud"
	"castai-agent/pkg/labels"
)

func TestProvider_RegisterCluster(t *testing.T) {
	t.Run("autodiscover cluster properties", func(t *testing.T) {
		require.NoError(t, os.Setenv("API_KEY", "123"))
		require.NoError(t, os.Setenv("API_URL", "test"))

		t.Cleanup(config.Reset)
		t.Cleanup(os.Clearenv)

		var objects []runtime.Object

		namespaceID := uuid.New()

		objects = append(objects, &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				UID:  k8stypes.UID(namespaceID.String()),
				Name: metav1.NamespaceSystem,
				Annotations: map[string]string{
					"addons.k8s.io/core.addons.k8s.io": `{"version":"1.4.0","channel":"s3://test-kops/test.k8s.local/addons/bootstrap-channel.yaml","manifestHash":"3ffe9ac576f9eec72e2bdfbd2ea17d56d9b17b90"}`,
				},
			},
		})

		// Simulate a large cluster with broken nodes.
		for i := 0; i < 100; i++ {
			objects = append(objects, &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "broken-" + strconv.Itoa(i),
					Labels: map[string]string{},
				},
			})
		}

		objects = append(objects, &v1.Node{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name: "normal",
				Labels: map[string]string{
					v1.LabelTopologyRegion: "us-east-1",
				},
			},
			Spec: v1.NodeSpec{
				ProviderID: "aws://us-east-1a/i-abcdefgh",
			},
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:   v1.NodeReady,
						Status: v1.ConditionTrue,
					},
				},
			},
		})

		clientset := fake.NewSimpleClientset(objects...)
		discoveryService := discovery.New(clientset, nil)

		p, err := New(logrus.New(), discoveryService)
		require.NoError(t, err)

		castaiclient := mock_castai.NewMockClient(t)

		registrationResp := &types.ClusterRegistration{
			ClusterID:      namespaceID.String(),
			OrganizationID: uuid.New().String(),
		}

		castaiclient.EXPECT().RegisterCluster(mock.Anything, &castai.RegisterClusterRequest{
			ID:   namespaceID,
			Name: "test.k8s.local",
			KOPS: &castai.KOPSParams{
				CSP:         string(cloud.AWS),
				Region:      "us-east-1",
				ClusterName: "test.k8s.local",
				StateStore:  "s3://test-kops",
			},
		}).Return(&castai.RegisterClusterResponse{Cluster: castai.Cluster{
			ID:             registrationResp.ClusterID,
			OrganizationID: registrationResp.OrganizationID,
		}}, nil)

		got, err := p.RegisterCluster(context.Background(), castaiclient)

		require.NoError(t, err)
		require.Equal(t, registrationResp, got)
	})

	t.Run("override properties from config", func(t *testing.T) {
		require.NoError(t, os.Setenv("API_KEY", "123"))
		require.NoError(t, os.Setenv("API_URL", "test"))
		require.NoError(t, os.Setenv("KOPS_CSP", "aws"))
		require.NoError(t, os.Setenv("KOPS_REGION", "us-east-1"))
		require.NoError(t, os.Setenv("KOPS_CLUSTER_NAME", "test.k8s.local"))
		require.NoError(t, os.Setenv("KOPS_STATE_STORE", "s3://test-kops"))

		t.Cleanup(config.Reset)
		t.Cleanup(os.Clearenv)

		namespaceID := uuid.New()
		namespace := &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				UID:  k8stypes.UID(namespaceID.String()),
				Name: metav1.NamespaceSystem,
				Annotations: map[string]string{
					"addons.k8s.io/core.addons.k8s.io": `{"version":"1.4.0","channel":"s3://test-kops/test.k8s.local/addons/bootstrap-channel.yaml","manifestHash":"3ffe9ac576f9eec72e2bdfbd2ea17d56d9b17b90"}`,
				},
			},
		}

		clientset := fake.NewSimpleClientset(namespace)
		discoveryService := discovery.New(clientset, nil)

		p, err := New(logrus.New(), discoveryService)
		require.NoError(t, err)

		castaiclient := mock_castai.NewMockClient(t)

		registrationResp := &types.ClusterRegistration{
			ClusterID:      namespaceID.String(),
			OrganizationID: uuid.New().String(),
		}

		castaiclient.EXPECT().RegisterCluster(mock.Anything, &castai.RegisterClusterRequest{
			ID:   namespaceID,
			Name: "test.k8s.local",
			KOPS: &castai.KOPSParams{
				CSP:         string(cloud.AWS),
				Region:      "us-east-1",
				ClusterName: "test.k8s.local",
				StateStore:  "s3://test-kops",
			},
		}).Return(&castai.RegisterClusterResponse{Cluster: castai.Cluster{
			ID:             registrationResp.ClusterID,
			OrganizationID: registrationResp.OrganizationID,
		}}, nil)

		got, err := p.RegisterCluster(context.Background(), castaiclient)

		require.NoError(t, err)
		require.Equal(t, registrationResp, got)
	})
}

func TestProvider_IsSpot(t *testing.T) {
	t.Run("castai managed spot nodes", func(t *testing.T) {
		node := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					labels.CastaiSpot: "true",
				},
			},
		}

		p := &Provider{}

		got, err := p.isSpot(context.Background(), node)

		require.NoError(t, err)
		require.True(t, got)
	})

	t.Run("kops instance group spot nodes", func(t *testing.T) {
		node := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					labels.KopsSpot: "true",
				},
			},
		}

		p := &Provider{}

		got, err := p.isSpot(context.Background(), node)

		require.NoError(t, err)
		require.True(t, got)
	})

	t.Run("aws spot nodes", func(t *testing.T) {
		node := &v1.Node{
			Spec: v1.NodeSpec{
				ProviderID: "aws:///eu-west-1a/instanceID",
			},
		}

		awsclient := mock_aws.NewMockClient(t)

		p := &Provider{
			csp:       cloud.AWS,
			awsClient: awsclient,
		}

		awsclient.EXPECT().GetInstancesByInstanceIDs(mock.Anything, []string{"instanceID"}).Return([]ec2_types.Instance{
			{
				InstanceLifecycle: ec2_types.InstanceLifecycleTypeSpot,
			},
		}, nil)

		got, err := p.isSpot(context.Background(), node)

		require.NoError(t, err)
		require.True(t, got)
	})

	t.Run("gcp spot nodes", func(t *testing.T) {
		node := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					gke.LabelPreemptible: "true",
				},
			},
		}

		p := &Provider{
			csp: cloud.GCP,
		}

		got, err := p.isSpot(context.Background(), node)

		require.NoError(t, err)
		require.True(t, got)
	})

	t.Run("non spot node", func(t *testing.T) {
		node := &v1.Node{
			Spec: v1.NodeSpec{
				ProviderID: "aws:///eu-west-1a/instanceID",
			},
		}

		awsclient := mock_aws.NewMockClient(t)

		p := &Provider{
			csp:       cloud.AWS,
			awsClient: awsclient,
		}

		awsclient.EXPECT().GetInstancesByInstanceIDs(mock.Anything, []string{"instanceID"}).Return([]ec2_types.Instance{
			{
				InstanceLifecycle: ec2_types.InstanceLifecycleTypeScheduled,
			},
		}, nil)

		got, err := p.isSpot(context.Background(), node)

		require.NoError(t, err)
		require.False(t, got)
	})
}
