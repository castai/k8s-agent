package discovery

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	fakedynamic "k8s.io/client-go/dynamic/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"

	"castai-agent/internal/services/controller/scheme"
	"castai-agent/pkg/cloud"
)

func TestServiceImpl_GetKubeSystemNamespaceID(t *testing.T) {
	namespaceID := uuid.New()
	objects := []runtime.Object{
		&v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				UID:  k8stypes.UID(namespaceID.String()),
				Name: metav1.NamespaceSystem,
			},
		},
	}

	clientset := fakeclientset.NewSimpleClientset(objects...)
	dyno := fakedynamic.NewSimpleDynamicClient(scheme.Scheme, objects...)

	s := New(clientset, dyno)

	id, err := s.GetKubeSystemNamespaceID(context.Background())

	require.NoError(t, err)
	require.Equal(t, namespaceID, *id)
}

func TestServiceImpl_GetCSPAndRegion(t *testing.T) {
	objects := []runtime.Object{
		&v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
				UID:  k8stypes.UID(uuid.New().String()),
			},
			Spec: v1.NodeSpec{
				ProviderID: "aws:///us-east-1a/i-123",
			},
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:   v1.NodeReady,
						Status: v1.ConditionTrue,
					},
				},
			},
		},
		&v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
				UID:  k8stypes.UID(uuid.New().String()),
				Labels: map[string]string{
					v1.LabelTopologyRegion: "us-east-1",
				},
			},
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:   v1.NodeReady,
						Status: v1.ConditionTrue,
					},
				},
			},
		},
	}

	clientset := fakeclientset.NewSimpleClientset(objects...)
	dyno := fakedynamic.NewSimpleDynamicClient(scheme.Scheme, objects...)

	s := New(clientset, dyno)

	csp, region, err := s.GetCSPAndRegion(context.Background())

	require.NoError(t, err)
	require.Equal(t, cloud.AWS, csp)
	require.Equal(t, "us-east-1", region)
}
