package discovery

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	fakedynamic "k8s.io/client-go/dynamic/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"

	"castai-agent/internal/services/controller/scheme"
)

func TestServiceImpl_GetKOPSClusterNameAndStateStore(t *testing.T) {
	objects := []runtime.Object{
		&v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				UID:  k8stypes.UID(uuid.New().String()),
				Name: metav1.NamespaceSystem,
				Annotations: map[string]string{
					"addons.k8s.io/core.addons.k8s.io": `{"version":"1.4.0","channel":"s3://test-kops/test.k8s.local/addons/bootstrap-channel.yaml","manifestHash":"3ffe9ac576f9eec72e2bdfbd2ea17d56d9b17b90"}`,
				},
			},
		},
	}

	clientset := fakeclientset.NewSimpleClientset(objects...)
	dyno := fakedynamic.NewSimpleDynamicClient(scheme.Scheme, objects...)

	s := New(clientset, dyno)

	clusterName, stateStore, err := s.GetKOPSClusterNameAndStateStore(context.Background(), logrus.New())

	require.NoError(t, err)
	require.Equal(t, "test.k8s.local", clusterName)
	require.Equal(t, "s3://test-kops", stateStore)
}
