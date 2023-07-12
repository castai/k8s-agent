package controller

import (
	"context"
	"errors"
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
	authorizationv1 "k8s.io/api/authorization/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	authfakev1 "k8s.io/client-go/kubernetes/typed/authorization/v1/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metrics_fake "k8s.io/metrics/pkg/client/clientset/versioned/fake"

	"castai-agent/internal/castai"
	mock_castai "castai-agent/internal/castai/mock"
	"castai-agent/internal/config"
	"castai-agent/internal/services/controller/delta"
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

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "poddisruptionbudgets",
			Namespace: v1.NamespaceDefault,
		},
	}
	pdbData, err := delta.Encode(pdb)
	require.NoError(t, err)

	hpa := &autoscalingv1.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "horizontalpodautoscalers",
			Namespace: v1.NamespaceDefault,
		},
	}
	hpaData, err := delta.Encode(hpa)
	require.NoError(t, err)

	csi := &storagev1.CSINode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "csinodes",
			Namespace: v1.NamespaceDefault,
		},
	}
	csiData, err := delta.Encode(csi)
	require.NoError(t, err)

	fakeSelfSubjectAccessReviewsClient := &authfakev1.FakeSelfSubjectAccessReviews{
		Fake: &authfakev1.FakeAuthorizationV1{
			Fake: &k8stesting.Fake{},
		},
	}

	// returns true for all requests to fakeSelfSubjectAccessReviewsClient
	fakeSelfSubjectAccessReviewsClient.Fake.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, &authorizationv1.SelfSubjectAccessReview{
			Status: authorizationv1.SubjectAccessReviewStatus{
				Allowed: true,
			},
		}, nil
	})

	clientset := fake.NewSimpleClientset(node, pod, pdb, hpa, csi)
	clientset.Fake.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: autoscalingv1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{
					Group: "autoscaling",
					Name:  "horizontalpodautoscalers",
					Kind:  "HorizontalPodAutoscaler",
					Verbs: []string{
						"list",
						"watch",
						"get",
					},
				},
			},
		},
		{
			GroupVersion: storagev1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{
					Group: "storage.k8s.io",
					Name:  "csinodes",
					Kind:  "CSINode",
					Verbs: []string{
						"list",
						"watch",
						"get",
					},
				},
			},
		},
	}

	metricsClient := metrics_fake.NewSimpleClientset()

	version.EXPECT().Full().Return("1.21+").MaxTimes(2)

	clusterID := uuid.New()

	var invocations int64

	castaiclient.EXPECT().
		SendDelta(gomock.Any(), clusterID.String(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ context.Context, clusterID string, d *castai.Delta) error {
			defer atomic.AddInt64(&invocations, 1)

			require.Equal(t, clusterID, d.ClusterID)
			require.Equal(t, "1.21+", d.ClusterVersion)
			require.True(t, d.FullSnapshot)
			require.Len(t, d.Items, 5)

			var actualValues []string
			for _, item := range d.Items {
				actualValues = append(actualValues, fmt.Sprintf("%s-%s-%v", item.Event, item.Kind, item.Data))
			}

			require.Contains(t, actualValues, fmt.Sprintf("%s-%s-%v", castai.EventAdd, "Node", nodeData))
			require.Contains(t, actualValues, fmt.Sprintf("%s-%s-%v", castai.EventAdd, "Pod", podData))
			require.Contains(t, actualValues, fmt.Sprintf("%s-%s-%v", castai.EventAdd, "PodDisruptionBudget", pdbData))
			require.Contains(t, actualValues, fmt.Sprintf("%s-%s-%v", castai.EventAdd, "HorizontalPodAutoscaler", hpaData))
			require.Contains(t, actualValues, fmt.Sprintf("%s-%s-%v", castai.EventAdd, "CSINode", csiData))

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
	ctrl := New(log, f, clientset.Discovery(), castaiclient, metricsClient, provider, clusterID.String(), &config.Controller{
		Interval:             15 * time.Second,
		PrepTimeout:          2 * time.Second,
		InitialSleepDuration: 10 * time.Millisecond,
	},
		version,
		agentVersion,
		NewHealthzProvider(defaultHealthzCfg, log),
		fakeSelfSubjectAccessReviewsClient,
	)
	f.Start(ctx.Done())

	go func() {
		require.NoError(t, ctrl.Run(ctx))
	}()

	// This adds the PDB resource to the discovery API, so that watcher can pick it up.
	clientset.Fake.Resources = append(clientset.Fake.Resources, &metav1.APIResourceList{
		GroupVersion: policyv1.SchemeGroupVersion.String(),
		APIResources: []metav1.APIResource{
			{
				Group: "policy",
				Name:  "poddisruptionbudgets",
				Kind:  "PodDisruptionBudget",
				Verbs: []string{"get", "list", "watch"},
			},
		},
	})

	wait.Until(func() {
		if atomic.LoadInt64(&invocations) >= 1 {
			cancel()
		}
	}, 10*time.Millisecond, ctx.Done())
}

func TestNew(t *testing.T) {
	t.Run("should not collect pod metrics when discovery api is unavailable", func(t *testing.T) {
		r := require.New(t)
		mockctrl := gomock.NewController(t)
		castaiclient := mock_castai.NewMockClient(mockctrl)
		version := mock_version.NewMockInterface(mockctrl)
		provider := mock_types.NewMockProvider(mockctrl)

		clientset := fake.NewSimpleClientset()
		clientset.Discovery().(*fakediscovery.FakeDiscovery).
			PrependReactor("get", "group",
				func(_ k8stesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, errors.New("some error")
				})
		metricsClient := metrics_fake.NewSimpleClientset()

		version.EXPECT().Full().Return("1.21+").MaxTimes(2)

		clusterID := uuid.New()
		agentVersion := &config.AgentVersion{Version: "1.2.3"}

		f := informers.NewSharedInformerFactory(clientset, 0)
		log := logrus.New()
		log.SetLevel(logrus.DebugLevel)
		ctrl := New(log, f, clientset.Discovery(), castaiclient, metricsClient, provider, clusterID.String(), &config.Controller{
			Interval:             15 * time.Second,
			PrepTimeout:          2 * time.Second,
			InitialSleepDuration: 10 * time.Millisecond,
		}, version, agentVersion, NewHealthzProvider(defaultHealthzCfg, log), clientset.AuthorizationV1().SelfSubjectAccessReviews())

		r.NotNil(ctrl)

		_, found := ctrl.informers[reflect.TypeOf(&v1beta1.PodMetrics{})]
		r.False(found, "pod metrics informer should not be registered if metrics api is not available")
	})
}
