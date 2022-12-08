//go:build !race
// +build !race

package controller

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"castai-agent/internal/castai"
	mock_castai "castai-agent/internal/castai/mock"
	"castai-agent/internal/config"
	mock_types "castai-agent/internal/services/providers/types/mock"
	mock_version "castai-agent/internal/services/version/mock"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	metrics_fake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

func TestController_ShouldKeepDeltaAfterDelete(t *testing.T) {
	mockctrl := gomock.NewController(t)
	castaiclient := mock_castai.NewMockClient(mockctrl)
	version := mock_version.NewMockInterface(mockctrl)
	provider := mock_types.NewMockProvider(mockctrl)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: v1.NamespaceDefault, Name: "pod1"}}
	podData, err := encode(pod)
	require.NoError(t, err)

	clientset := fake.NewSimpleClientset()
	metricsClient := metrics_fake.NewSimpleClientset()
	f := informers.NewSharedInformerFactory(clientset, 0)

	version.EXPECT().MinorInt().Return(19).MaxTimes(2)
	version.EXPECT().Full().Return("1.19+").MaxTimes(2)

	clusterID := uuid.New()
	log := logrus.New()

	var invocations int64

	// initial full snapshot
	castaiclient.EXPECT().
		SendDelta(gomock.Any(), clusterID.String(), gomock.Any()).
		DoAndReturn(func(_ context.Context, clusterID string, d *castai.Delta) error {
			defer atomic.AddInt64(&invocations, 1)

			require.Equal(t, clusterID, d.ClusterID)
			require.Equal(t, "1.19+", d.ClusterVersion)
			require.True(t, d.FullSnapshot)
			require.Len(t, d.Items, 0)

			_, err := clientset.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
			require.NoError(t, err)

			return nil
		})

	// first delta add pod - fail and trigger pod delete
	castaiclient.EXPECT().
		SendDelta(gomock.Any(), clusterID.String(), gomock.Any()).
		DoAndReturn(func(_ context.Context, clusterID string, d *castai.Delta) error {
			defer atomic.AddInt64(&invocations, 1)

			require.Equal(t, clusterID, d.ClusterID)
			require.Equal(t, "1.19+", d.ClusterVersion)
			require.False(t, d.FullSnapshot)
			require.Len(t, d.Items, 1)

			var actualValues []string
			for _, item := range d.Items {
				actualValues = append(actualValues, fmt.Sprintf("%s-%s-%s", item.Event, item.Kind, item.Data))
			}

			require.Contains(t, actualValues, fmt.Sprintf("%s-%s-%s", castai.EventAdd, "Pod", podData))

			err := clientset.CoreV1().Pods("default").Delete(ctx, pod.Name, metav1.DeleteOptions{})
			require.NoError(t, err)

			return fmt.Errorf("testError")
		})

	// second attempt to send data when pod delete is received
	castaiclient.EXPECT().
		SendDelta(gomock.Any(), clusterID.String(), gomock.Any()).
		DoAndReturn(func(_ context.Context, clusterID string, d *castai.Delta) error {
			defer atomic.AddInt64(&invocations, 1)

			require.Equal(t, clusterID, d.ClusterID)
			require.Equal(t, "1.19+", d.ClusterVersion)
			require.False(t, d.FullSnapshot)
			require.Len(t, d.Items, 1)

			var actualValues []string
			for _, item := range d.Items {
				actualValues = append(actualValues, fmt.Sprintf("%s-%s-%s", item.Event, item.Kind, item.Data))
			}

			require.Contains(t, actualValues, fmt.Sprintf("%s-%s-%s", castai.EventDelete, "Pod", podData))

			return nil
		})

	agentVersion := &config.AgentVersion{Version: "1.2.3"}
	castaiclient.EXPECT().ExchangeAgentTelemetry(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().
		Return(&castai.AgentTelemetryResponse{}, nil).
		Do(func(ctx context.Context, clusterID string, req *castai.AgentTelemetryRequest) {
			require.Equalf(t, "1.2.3", req.AgentVersion, "got request: %+v", req)
		})

	log.SetLevel(logrus.DebugLevel)
	ctrl, _ := New(log, f, clientset.Discovery(), castaiclient, metricsClient, provider, clusterID.String(), &config.Controller{
		Interval:             2 * time.Second,
		PrepTimeout:          2 * time.Second,
		InitialSleepDuration: 10 * time.Millisecond,
	}, version, agentVersion, NewHealthzProvider(defaultHealthzCfg, log))

	f.Start(ctx.Done())

	go func() {
		require.NoError(t, ctrl.Run(ctx))
	}()

	wait.Until(func() {
		if atomic.LoadInt64(&invocations) >= 3 {
			cancel()
		}
	}, 10*time.Millisecond, ctx.Done())
}
