package controller

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"castai-agent/internal/castai"
	mock_castai "castai-agent/internal/castai/mock"
	mock_types "castai-agent/internal/services/providers/types/mock"
	mock_version "castai-agent/internal/services/version/mock"
	"castai-agent/pkg/labels"
)

func Test(t *testing.T) {
	mockctrl := gomock.NewController(t)
	castaiclient := mock_castai.NewMockClient(mockctrl)
	version := mock_version.NewMockInterface(mockctrl)
	provider := mock_types.NewMockProvider(mockctrl)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{}}}
	expectedNode := node.DeepCopy()
	expectedNode.Labels[labels.Spot] = "true"
	nodeData, err := encode(expectedNode)
	require.NoError(t, err)

	pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: v1.NamespaceDefault, Name: "pod1"}}
	podData, err := encode(pod)
	require.NoError(t, err)

	clientset := fake.NewSimpleClientset(node, pod)

	version.EXPECT().MinorInt().Return(19)
	version.EXPECT().Full().Return("1.19+")

	clusterID := uuid.New()

	var invocations int64

	castaiclient.EXPECT().
		SendDelta(gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ context.Context, d *castai.Delta) error {
			defer atomic.AddInt64(&invocations, 1)

			require.Equal(t, clusterID.String(), d.ClusterID)
			require.Equal(t, "1.19+", d.ClusterVersion)
			require.True(t, d.Resync)
			require.Len(t, d.Items, 2)

			var actualValues []string
			for _, item := range d.Items {
				actualValues = append(actualValues, fmt.Sprintf("%s-%s-%s", item.Event, item.Kind, item.Data))
			}

			require.Contains(t, actualValues, fmt.Sprintf("%s-%s-%s", castai.EventAdd, "Node", nodeData))
			require.Contains(t, actualValues, fmt.Sprintf("%s-%s-%s", castai.EventAdd, "Pod", podData))

			return nil
		})

	castaiclient.EXPECT().GetAgentCfg(gomock.Any(), gomock.Any()).AnyTimes().Return(&castai.AgentCfgResponse{}, nil)
	provider.EXPECT().IsSpot(gomock.Any(), node).Return(true, nil)

	f := informers.NewSharedInformerFactory(clientset, 0)
	ctrl := New(logrus.New(), f, castaiclient, provider, clusterID.String(), 15*time.Second, 100*time.Millisecond, version)
	f.Start(ctx.Done())

	go ctrl.Run(ctx)

	wait.Until(func() {
		if atomic.LoadInt64(&invocations) >= 1 {
			cancel()
		}
	}, 10*time.Millisecond, ctx.Done())
}
