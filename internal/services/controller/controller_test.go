package controller

import (
	"context"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	metrics_fake "k8s.io/metrics/pkg/client/clientset/versioned/fake"

	"castai-agent/internal/castai"
	mock_castai "castai-agent/internal/castai/mock"
	"castai-agent/internal/config"
	"castai-agent/internal/services/controller/delta"
	mock_workqueue "castai-agent/internal/services/controller/mock"
	mock_types "castai-agent/internal/services/providers/types/mock"
	mock_version "castai-agent/internal/services/version/mock"
	"castai-agent/pkg/labels"
)

var defaultHealthzCfg = config.Config{Controller: &config.Controller{
	Interval:                       15 * time.Second,
	PrepTimeout:                    10 * time.Minute,
	InitialSleepDuration:           30 * time.Second,
	HealthySnapshotIntervalLimit:   10 * time.Minute,
	InitializationTimeoutExtension: 5 * time.Minute,
}}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(
		m,
		goleak.IgnoreTopFunction("k8s.io/klog/v2.(*loggingT).flushDaemon"),
		goleak.IgnoreTopFunction("k8s.io/client-go/util/workqueue.(*Type).updateUnfinishedWorkLoop"),
	)
}

func TestController_HappyPath(t *testing.T) {
	mockctrl := gomock.NewController(t)
	castaiclient := mock_castai.NewMockClient(mockctrl)
	version := mock_version.NewMockInterface(mockctrl)
	provider := mock_types.NewMockProvider(mockctrl)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{}}}
	expectedNode := node.DeepCopy()
	expectedNode.Labels[labels.CastaiFakeSpot] = "true"
	nodeData, err := delta.Encode(expectedNode)
	require.NoError(t, err)

	pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: v1.NamespaceDefault, Name: "pod1"}}
	podData, err := delta.Encode(pod)
	require.NoError(t, err)

	clientset := fake.NewSimpleClientset(node, pod)
	metricsClient := metrics_fake.NewSimpleClientset()

	version.EXPECT().MinorInt().Return(19).MaxTimes(2)
	version.EXPECT().Full().Return("1.19+").MaxTimes(2)

	clusterID := uuid.New()

	var invocations int64

	castaiclient.EXPECT().
		SendDelta(gomock.Any(), clusterID.String(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ context.Context, clusterID string, d *castai.Delta) error {
			defer atomic.AddInt64(&invocations, 1)

			require.Equal(t, clusterID, d.ClusterID)
			require.Equal(t, "1.19+", d.ClusterVersion)
			require.True(t, d.FullSnapshot)
			require.Len(t, d.Items, 2)

			var actualValues []string
			for _, item := range d.Items {
				actualValues = append(actualValues, fmt.Sprintf("%s-%s-%v", item.Event, item.Kind, item.Data))
			}

			require.Contains(t, actualValues, fmt.Sprintf("%s-%s-%v", castai.EventAdd, "Node", nodeData))
			require.Contains(t, actualValues, fmt.Sprintf("%s-%s-%v", castai.EventAdd, "Pod", podData))

			return nil
		})

	agentVersion := &config.AgentVersion{Version: "1.2.3"}
	castaiclient.EXPECT().ExchangeAgentTelemetry(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().
		Return(&castai.AgentTelemetryResponse{}, nil).
		Do(func(ctx context.Context, clusterID string, req *castai.AgentTelemetryRequest) {
			require.Equalf(t, "1.2.3", req.AgentVersion, "got request: %+v", req)
		})

	provider.EXPECT().FilterSpot(gomock.Any(), []*v1.Node{node}).Return([]*v1.Node{node}, nil)

	f := informers.NewSharedInformerFactory(clientset, 0)
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	ctrl, _ := New(log, f, clientset.Discovery(), castaiclient, metricsClient, provider, clusterID.String(), &config.Controller{
		Interval:             15 * time.Second,
		PrepTimeout:          2 * time.Second,
		InitialSleepDuration: 10 * time.Millisecond,
	}, version, agentVersion, NewHealthzProvider(defaultHealthzCfg, log))
	f.Start(ctx.Done())

	go func() {
		require.NoError(t, ctrl.Run(ctx))
	}()

	wait.Until(func() {
		if atomic.LoadInt64(&invocations) >= 1 {
			cancel()
		}
	}, 10*time.Millisecond, ctx.Done())
}

func TestEventHandlers(t *testing.T) {

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod",
			Namespace: v1.NamespaceDefault,
		},
	}

	hpa := &autoscalingv1.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "horizontalpodautoscaler",
			Namespace: v1.NamespaceDefault,
		},
	}

	items := []object{pod, hpa}

	t.Run("should handle add events", func(t *testing.T) {
		queue := mock_workqueue.NewMockInterface(gomock.NewController(t))

		c := &Controller{
			log:   logrus.New(),
			queue: queue,
		}

		for i := range items {
			handlers := c.createEventHandlers(c.log, reflect.TypeOf(items[i]))
			queue.EXPECT().Add(&item{
				obj:   items[i],
				event: eventAdd,
			})
			handlers.OnAdd(items[i])
		}
	})

	t.Run("should handle update events", func(t *testing.T) {
		queue := mock_workqueue.NewMockInterface(gomock.NewController(t))

		c := &Controller{
			log:   logrus.New(),
			queue: queue,
		}

		updates := make([]object, len(items))
		copy(updates, items)
		for i := range updates {
			updates[i].SetLabels(map[string]string{"a": "b"})
		}

		for i := range items {
			handlers := c.createEventHandlers(c.log, reflect.TypeOf(items[i]))
			queue.EXPECT().Add(&item{
				obj:   items[i],
				event: eventUpdate,
			})
			handlers.OnUpdate(items[i], updates[i])
		}
	})

	t.Run("should handle delete events", func(t *testing.T) {
		queue := mock_workqueue.NewMockInterface(gomock.NewController(t))

		c := &Controller{
			log:   logrus.New(),
			queue: queue,
		}

		for i := range items {
			handlers := c.createEventHandlers(c.log, reflect.TypeOf(items[i]))
			queue.EXPECT().Add(&item{
				obj:   items[i],
				event: eventDelete,
			})

			handlers.OnDelete(items[i])
		}
	})

	t.Run("should handle cache.DeletedFinalStateUnknown events", func(t *testing.T) {
		queue := mock_workqueue.NewMockInterface(gomock.NewController(t))

		c := &Controller{
			log:   logrus.New(),
			queue: queue,
		}

		handlers := c.createEventHandlers(c.log, reflect.TypeOf(&v1.Pod{}))

		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod",
				Namespace: v1.NamespaceDefault,
			},
		}

		queue.EXPECT().Add(&item{
			obj:   pod,
			event: eventDelete,
		})

		handlers.OnDelete(cache.DeletedFinalStateUnknown{
			Key: "default/pod",
			Obj: pod,
		})
	})
}
