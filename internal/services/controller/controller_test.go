package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	argorollouts "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	appsv1 "k8s.io/api/apps/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	dynamic_fake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	authfakev1 "k8s.io/client-go/kubernetes/typed/authorization/v1/fake"
	k8stesting "k8s.io/client-go/testing"
	metrics_fake "k8s.io/metrics/pkg/client/clientset/versioned/fake"

	"castai-agent/internal/castai"
	mock_castai "castai-agent/internal/castai/mock"
	"castai-agent/internal/config"
	"castai-agent/internal/services/controller/crd"
	"castai-agent/internal/services/controller/delta"
	"castai-agent/internal/services/controller/knowngv"
	mock_discovery "castai-agent/internal/services/controller/mock/discovery"
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

type sampleObject struct {
	GV       schema.GroupVersion
	Kind     string
	Resource string
	Data     []byte
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(
		m,
		goleak.IgnoreTopFunction("k8s.io/klog/v2.(*loggingT).flushDaemon"),
		goleak.IgnoreTopFunction("k8s.io/client-go/util/workqueue.(*Type).updateUnfinishedWorkLoop"),
	)
}

func TestController_ShouldReceiveDeltasBasedOnAvailableResources(t *testing.T) {
	tests := map[string]struct {
		expectedReceivedObjectsCount int
		apiResourceError             error
	}{
		"All supported objects are found and received in delta": {
			expectedReceivedObjectsCount: 21,
		},
		"when fetching api resources produces multiple errors should exclude those resources": {
			apiResourceError: fmt.Errorf("unable to retrieve the complete list of server APIs: %v:"+
				"stale GroupVersion discovery: some error,%v: another error",
				policyv1.SchemeGroupVersion.String(), storagev1.SchemeGroupVersion.String()),
			expectedReceivedObjectsCount: 19,
		},
		"when fetching api resources produces single error should exclude that resource": {
			apiResourceError: fmt.Errorf("unable to retrieve the complete list of server APIs: %v:"+
				"stale GroupVersion discovery: some error", storagev1.SchemeGroupVersion.String()),
			expectedReceivedObjectsCount: 20,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			utilruntime.Must(datadoghqv1alpha1.SchemeBuilder.AddToScheme(scheme))
			utilruntime.Must(argorollouts.SchemeBuilder.AddToScheme(scheme))
			utilruntime.Must(crd.SchemeBuilder.AddToScheme(scheme))

			mockctrl := gomock.NewController(t)
			castaiclient := mock_castai.NewMockClient(mockctrl)
			version := mock_version.NewMockInterface(mockctrl)
			provider := mock_types.NewMockProvider(mockctrl)
			objectsData, clientset, dynamicClient := loadInitialHappyPathData(t, scheme)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

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

			metricsClient := metrics_fake.NewSimpleClientset()
			log := logrus.New()
			log.SetLevel(logrus.DebugLevel)

			version.EXPECT().Full().Return("1.21+").MaxTimes(3)
			agentVersion := &config.AgentVersion{Version: "1.2.3"}

			clusterID := uuid.New()
			var mockDiscovery *mock_discovery.MockDiscoveryInterface
			_, apiResources, _ := clientset.Discovery().ServerGroupsAndResources()
			if tt.apiResourceError != nil {
				mockDiscovery = mock_discovery.NewMockDiscoveryInterface(mockctrl)
				errors := extractGroupVersionsFromApiResourceError(log, tt.apiResourceError)
				apiResources = lo.Filter(apiResources, func(apiResource *metav1.APIResourceList, _ int) bool {
					gv, _ := schema.ParseGroupVersion(apiResource.GroupVersion)
					return !errors[gv]
				})

				// filter expected data based on available resources
				objectsData = lo.Filter(objectsData, func(obj sampleObject, _ int) bool {
					_, found := lo.Find(apiResources, func(r *metav1.APIResourceList) bool {
						return r.APIResources[0].Name == obj.Resource
					})
					return found
				})
				mockDiscovery.EXPECT().ServerGroupsAndResources().Return([]*metav1.APIGroup{}, apiResources, tt.apiResourceError).AnyTimes()
			}
			var invocations int64

			castaiclient.EXPECT().
				SendDelta(gomock.Any(), clusterID.String(), gomock.Any()).AnyTimes().
				DoAndReturn(func(_ context.Context, clusterID string, d *castai.Delta) error {
					defer atomic.AddInt64(&invocations, 1)

					require.Equal(t, clusterID, d.ClusterID)
					require.Equal(t, "1.21+", d.ClusterVersion)
					require.Equal(t, "1.2.3", d.AgentVersion)
					require.True(t, d.FullSnapshot)
					require.Equal(t, tt.expectedReceivedObjectsCount, len(d.Items), "number of items in delta")

					for _, expected := range objectsData {
						expectedGVString := expected.GV.String()
						actual, found := lo.Find(d.Items, func(item *castai.DeltaItem) bool {
							return item.Event == castai.EventAdd &&
								item.Kind == expected.Kind &&
								item.Data != nil &&
								strings.Contains(string(*item.Data), expectedGVString) // Hacky but OK given this is for testing purposes.
						})
						require.True(t, found)
						require.NotNil(t, actual.Data)
						require.JSONEq(t, string(expected.Data), string(*actual.Data))
					}

					return nil
				})

			castaiclient.EXPECT().ExchangeAgentTelemetry(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().
				Return(&castai.AgentTelemetryResponse{}, nil).
				Do(func(ctx context.Context, clusterID string, req *castai.AgentTelemetryRequest) {
					require.Equalf(t, "1.2.3", req.AgentVersion, "got request: %+v", req)
				})

			node := &v1.Node{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Node",
					APIVersion: v1.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:   "node1",
					Labels: map[string]string{},
				},
			}
			provider.EXPECT().FilterSpot(gomock.Any(), []*v1.Node{node}).Return([]*v1.Node{node}, nil)

			ctrl := New(
				log,
				clientset,
				dynamicClient,
				castaiclient,
				metricsClient,
				provider,
				clusterID.String(),
				&config.Controller{
					Interval:             15 * time.Second,
					PrepTimeout:          2 * time.Second,
					InitialSleepDuration: 10 * time.Millisecond,
					ConfigMapNamespaces:  []string{v1.NamespaceDefault},
				},
				version,
				agentVersion,
				NewHealthzProvider(defaultHealthzCfg, log),
				fakeSelfSubjectAccessReviewsClient,
				"castai-agent",
			)

			if mockDiscovery != nil {
				ctrl.discovery = mockDiscovery
			}

			ctrl.Start(ctx.Done())

			go func() {
				require.NoError(t, ctrl.Run(ctx))
			}()

			wait.Until(func() {
				if atomic.LoadInt64(&invocations) >= 1 {
					cancel()
				}
			}, 10*time.Millisecond, ctx.Done())
		})
	}
}

func TestController_ApiResourcesErrorProcessing(t *testing.T) {
	err := fmt.Errorf("unable to retrieve the complete list of server APIs: external.metrics.k8s.io/v1beta1: stale GroupVersion discovery: external.metrics.k8s.io/v1beta1,external.metrics.k8s.io/v2beta2: stale GroupVersion discovery: external.metrics.k8s.io/v2beta2")
	val := extractGroupVersionsFromApiResourceError(logrus.New(), err)
	require.Len(t, val, 2)
	require.True(t, val[schema.GroupVersion{
		Group:   "external.metrics.k8s.io",
		Version: "v1beta1",
	}])
	require.True(t, val[schema.GroupVersion{
		Group:   "external.metrics.k8s.io",
		Version: "v2beta2",
	}])
}

func TestController_ShouldSendByInterval(t *testing.T) {
	tt := []struct {
		name          string
		sendInterval  time.Duration
		sendDurations []time.Duration
		checkAfter    time.Duration
		wantSends     int64
	}{
		{
			name:         "should trigger all sends when none exceed allocated intervals",
			sendInterval: 300 * time.Millisecond,
			sendDurations: []time.Duration{
				// Send by 300ms
				250 * time.Millisecond,
				// ...by 600ms
				250 * time.Millisecond,
				// ...by 900ms
				250 * time.Millisecond,
				// ...by 1200ms
				250 * time.Millisecond,
			},
			checkAfter: 1200 * time.Millisecond,
			wantSends:  4,
		},
		{
			name:         "should trigger all sends when previous send exceeds one interval",
			sendInterval: 300 * time.Millisecond,
			sendDurations: []time.Duration{
				// At 0, 300: idle, previous still sending
				// Send by 600 ms
				450 * time.Millisecond,
				// Send by 600 ms too, since we expect ticker to fire events even between ticks
				50 * time.Millisecond,
			},
			checkAfter: 600 * time.Millisecond,
			wantSends:  2,
		},
		{
			name:         "should trigger all sends when previous send exceeds two intervals",
			sendInterval: 300 * time.Millisecond,
			sendDurations: []time.Duration{
				// At 0, 300, 600ms: idle, previous still sending
				// Send by 900 ms
				650 * time.Millisecond,
				// Send by 900 ms too, since we expect ticker to fire events even between ticks
				150 * time.Millisecond,
			},
			checkAfter: 900 * time.Millisecond,
			wantSends:  2,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			mockctrl := gomock.NewController(t)
			castaiclient := mock_castai.NewMockClient(mockctrl)
			version := mock_version.NewMockInterface(mockctrl)
			provider := mock_types.NewMockProvider(mockctrl)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: v1.NamespaceDefault, Name: "pod1"}}
			_, err := delta.Encode(pod)
			r := require.New(t)
			r.NoError(err)

			clientset := fake.NewSimpleClientset()
			metricsClient := metrics_fake.NewSimpleClientset()
			dynamicClient := dynamic_fake.NewSimpleDynamicClient(runtime.NewScheme())

			version.EXPECT().Full().Return("1.21+").AnyTimes()

			clusterID := uuid.New()
			log := logrus.New()

			var gotSends atomic.Int64
			var wg sync.WaitGroup
			wg.Add(len(tc.sendDurations))
			var sentFirstTickNotification atomic.Bool

			var firstTickAt time.Time
			var lastSentAt time.Time

			for _, sendDuration := range tc.sendDurations {
				castaiclient.EXPECT().
					SendDelta(gomock.Any(), clusterID.String(), gomock.Any()).
					DoAndReturn(func(_ context.Context, clusterID string, d *castai.Delta) error {
						if !sentFirstTickNotification.Load() {
							firstTickAt = time.Now()
							sentFirstTickNotification.Store(true)
						}
						time.Sleep(sendDuration)
						if gotSends.Add(1) == tc.wantSends {
							lastSentAt = time.Now()
						}
						wg.Done()
						return nil
					})
			}

			agentVersion := &config.AgentVersion{Version: "1.2.3"}
			castaiclient.EXPECT().ExchangeAgentTelemetry(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().
				Return(&castai.AgentTelemetryResponse{}, nil).
				Do(func(ctx context.Context, clusterID string, req *castai.AgentTelemetryRequest) {
					r.Equalf("1.2.3", req.AgentVersion, "got request: %+v", req)
				})

			log.SetLevel(logrus.DebugLevel)
			ctrl := New(
				log,
				clientset,
				dynamicClient,
				castaiclient,
				metricsClient,
				provider,
				clusterID.String(),
				&config.Controller{
					Interval:             tc.sendInterval,
					PrepTimeout:          300 * time.Millisecond,
					InitialSleepDuration: 10 * time.Millisecond,
				},
				version,
				agentVersion,
				NewHealthzProvider(defaultHealthzCfg, log),
				clientset.AuthorizationV1().SelfSubjectAccessReviews(),
				"castai-agent",
			)

			ctrl.Start(ctx.Done())

			go func() {
				r.NoError(ctrl.Run(ctx))
			}()
			wg.Wait()

			r.Equal(tc.wantSends, gotSends.Load(), "sends don't match, failing at: %s", time.Now())
			elapsed := lastSentAt.Sub(firstTickAt)
			deadline := tc.checkAfter
			r.LessOrEqualf(elapsed, deadline, "elapsed time is greater than deadline: %s > %s", elapsed, deadline)

			wait.Until(func() {
				if gotSends.Load() == int64(len(tc.sendDurations)) {
					cancel()
				}
			}, 10*time.Millisecond, ctx.Done())
		})
	}

}

func TestController_ShouldKeepDeltaAfterDelete(t *testing.T) {
	mockctrl := gomock.NewController(t)
	castaiclient := mock_castai.NewMockClient(mockctrl)
	version := mock_version.NewMockInterface(mockctrl)
	provider := mock_types.NewMockProvider(mockctrl)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pod := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: v1.NamespaceDefault,
			Name:      "pod1",
		},
	}
	podData, err := delta.Encode(pod)
	require.NoError(t, err)

	clientset := fake.NewSimpleClientset()
	metricsClient := metrics_fake.NewSimpleClientset()
	dynamicClient := dynamic_fake.NewSimpleDynamicClient(runtime.NewScheme())

	version.EXPECT().Full().Return("1.21+").MaxTimes(3)

	agentVersion := &config.AgentVersion{Version: "1.2.3"}

	clusterID := uuid.New()
	log := logrus.New()

	var invocations int64

	// initial full snapshot
	castaiclient.EXPECT().
		SendDelta(gomock.Any(), clusterID.String(), gomock.Any()).
		DoAndReturn(func(_ context.Context, clusterID string, d *castai.Delta) error {
			defer atomic.AddInt64(&invocations, 1)

			require.Equal(t, clusterID, d.ClusterID)
			require.Equal(t, "1.21+", d.ClusterVersion)
			require.Equal(t, "1.2.3", d.AgentVersion)
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
			require.Equal(t, "1.21+", d.ClusterVersion)
			require.Equal(t, "1.2.3", d.AgentVersion)
			require.False(t, d.FullSnapshot)
			require.Len(t, d.Items, 1)

			actualItem, found := lo.Find(d.Items, func(item *castai.DeltaItem) bool {
				return item.Event == castai.EventAdd && item.Kind == "Pod"
			})
			require.True(t, found)
			require.NotNil(t, actualItem.Data)
			require.JSONEq(t, string(*podData), string(*actualItem.Data))

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
			require.Equal(t, "1.21+", d.ClusterVersion)
			require.Equal(t, "1.2.3", d.AgentVersion)
			require.False(t, d.FullSnapshot)
			require.Len(t, d.Items, 1)

			actualItem, found := lo.Find(d.Items, func(item *castai.DeltaItem) bool {
				return item.Event == castai.EventDelete && item.Kind == "Pod"
			})
			require.True(t, found)
			require.NotNil(t, actualItem.Data)
			require.JSONEq(t, string(*podData), string(*actualItem.Data))

			return nil
		})

	castaiclient.EXPECT().ExchangeAgentTelemetry(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().
		Return(&castai.AgentTelemetryResponse{}, nil).
		Do(func(ctx context.Context, clusterID string, req *castai.AgentTelemetryRequest) {
			require.Equalf(t, "1.2.3", req.AgentVersion, "got request: %+v", req)
		})

	log.SetLevel(logrus.DebugLevel)
	ctrl := New(
		log,
		clientset,
		dynamicClient,
		castaiclient,
		metricsClient,
		provider,
		clusterID.String(),
		&config.Controller{
			Interval:             2 * time.Second,
			PrepTimeout:          2 * time.Second,
			InitialSleepDuration: 10 * time.Millisecond,
		},
		version,
		agentVersion,
		NewHealthzProvider(defaultHealthzCfg, log),
		clientset.AuthorizationV1().SelfSubjectAccessReviews(),
		"castai-agent",
	)

	ctrl.Start(ctx.Done())

	go func() {
		require.NoError(t, ctrl.Run(ctx))
	}()

	wait.Until(func() {
		if atomic.LoadInt64(&invocations) >= 3 {
			cancel()
		}
	}, 10*time.Millisecond, ctx.Done())
}

func loadInitialHappyPathData(t *testing.T, scheme *runtime.Scheme) ([]sampleObject, *fake.Clientset, *dynamic_fake.FakeDynamicClient) {
	provisionersGvr := knowngv.KarpenterCoreV1Alpha5.WithResource("provisioners")
	machinesGvr := knowngv.KarpenterCoreV1Alpha5.WithResource("machines")
	awsNodeTemplatesGvr := knowngv.KarpenterV1Alpha1.WithResource("awsnodetemplates")
	nodePoolsGvr := knowngv.KarpenterCoreV1Beta1.WithResource("nodepools")
	nodeClaimsGvr := knowngv.KarpenterCoreV1Beta1.WithResource("nodeclaims")
	ec2NodeClassesGvr := knowngv.KarpenterV1Beta1.WithResource("ec2nodeclasses")
	datadogExtendedDSReplicaSetsGvr := datadoghqv1alpha1.GroupVersion.WithResource("extendeddaemonsetreplicasets")

	node := &v1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "node1", Labels: map[string]string{},
		},
	}
	expectedNode := node.DeepCopy()
	expectedNode.Labels[labels.CastaiFakeSpot] = "true"
	nodeData := asJson(t, expectedNode)

	pod := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: v1.NamespaceDefault, Name: "pod1",
		},
	}
	podData := asJson(t, pod)

	cfgMap := &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: v1.NamespaceDefault,
			Name:      "cfg1",
		},
		Data: map[string]string{
			"field1": "value1",
		},
	}
	cfgMapData := asJson(t, cfgMap)

	pdb := &policyv1.PodDisruptionBudget{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PodDisruptionBudget",
			APIVersion: policyv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "poddisruptionbudgets",
			Namespace: v1.NamespaceDefault,
		},
	}
	pdbData := asJson(t, pdb)

	hpa := &autoscalingv1.HorizontalPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HorizontalPodAutoscaler",
			APIVersion: autoscalingv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "horizontalpodautoscalers",
			Namespace: v1.NamespaceDefault,
		},
	}
	hpaData := asJson(t, hpa)

	csi := &storagev1.CSINode{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CSINode",
			APIVersion: storagev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "csinodes",
			Namespace: v1.NamespaceDefault,
		},
	}
	csiData := asJson(t, csi)

	provisionersData := []byte(`{
		"kind": "Provisioner",
		"apiVersion": "karpenter.sh/v1alpha5",
		"metadata": {
			"name": "provisioner",
			"namespace": "default"			
		}
	}`)

	machinesData := []byte(`{
		"kind": "Machine",
		"apiVersion": "karpenter.sh/v1alpha5",
		"metadata": {
			"name": "machine",
			"namespace": "default"			
		}
	}`)

	awsNodeTemplatesData := []byte(`{
		"kind": "AWSNodeTemplate",
		"apiVersion": "karpenter.k8s.aws/v1alpha1",
		"metadata": {
			"name": "awsnodetemplate",
			"namespace": "default"			
		}
	}`)

	nodePoolsData := []byte(`{
		"kind": "NodePool",
		"apiVersion": "karpenter.sh/v1beta1",
		"metadata": {
			"name": "nodepool",
			"namespace": "default"			
		}
	}`)

	nodeClaimsData := []byte(`{
		"kind": "NodeClaim",
		"apiVersion": "karpenter.sh/v1beta1",
		"metadata": {
			"name": "nodeclaim",
			"namespace": "default"			
		}
	}`)

	ec2NodeClassesData := []byte(`{
		"kind": "EC2NodeClass",
		"apiVersion": "karpenter.k8s.aws/v1beta1",
		"metadata": {
			"name": "ec2nodeclass",
			"namespace": "default"			
		}
	}`)

	datadogExtendedDSReplicaSet := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ExtendedDaemonSetReplicaSet",
			APIVersion: datadogExtendedDSReplicaSetsGvr.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      datadogExtendedDSReplicaSetsGvr.Resource,
			Namespace: v1.NamespaceDefault,
		},
	}

	datadogExtendedDSReplicaSetData := asJson(t, datadogExtendedDSReplicaSet)

	rollout := &argorollouts.Rollout{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Rollout",
			APIVersion: argorollouts.RolloutGVR.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      argorollouts.RolloutGVR.Resource,
			Namespace: v1.NamespaceDefault,
		},
	}

	rolloutData := asJson(t, rollout)

	recommendation := &crd.Recommendation{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Recommendation",
			APIVersion: crd.SchemaGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      crd.RecommendationGVR.Resource,
			Namespace: v1.NamespaceDefault,
		},
	}

	recommendationData := asJson(t, recommendation)

	ingress := &networkingv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: networkingv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: v1.NamespaceDefault,
			Name:      "ingress",
		},
	}
	ingressData := asJson(t, ingress)

	netpolicy := &networkingv1.NetworkPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NetworkPolicy",
			APIVersion: networkingv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: v1.NamespaceDefault,
			Name:      "netpolicy",
		},
	}
	netpolicyData := asJson(t, netpolicy)

	role := &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Role",
			APIVersion: rbacv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: v1.NamespaceDefault,
			Name:      "role",
		},
	}
	roleData := asJson(t, role)

	roleBinding := &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "RoleBinding",
			APIVersion: rbacv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: v1.NamespaceDefault,
			Name:      "rolebinding",
		},
	}
	roleBindingData := asJson(t, roleBinding)

	clusterRole := &rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRole",
			APIVersion: rbacv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: v1.NamespaceDefault,
			Name:      "clusterrole",
		},
	}
	clusterRoleData := asJson(t, clusterRole)

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRoleBinding",
			APIVersion: rbacv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: v1.NamespaceDefault,
			Name:      "clusterrolebinding",
		},
	}
	clusterRoleBindingData := asJson(t, clusterRoleBinding)

	clientset := fake.NewSimpleClientset(
		node,
		pod,
		cfgMap,
		pdb,
		hpa,
		csi,
		ingress,
		netpolicy,
		role,
		roleBinding,
		clusterRole,
		clusterRoleBinding,
	)
	runtimeObjects := []runtime.Object{
		unstructuredFromJson(t, provisionersData),
		unstructuredFromJson(t, machinesData),
		unstructuredFromJson(t, awsNodeTemplatesData),
		unstructuredFromJson(t, nodePoolsData),
		unstructuredFromJson(t, nodeClaimsData),
		unstructuredFromJson(t, ec2NodeClassesData),
		datadogExtendedDSReplicaSet,
		rollout,
		recommendation,
	}
	dynamicClient := dynamic_fake.NewSimpleDynamicClient(scheme, runtimeObjects...)
	clientset.Fake.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: autoscalingv1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{
					Group: "autoscaling",
					Name:  "horizontalpodautoscalers",
					Kind:  "HorizontalPodAutoscaler",
					Verbs: []string{"get", "list", "watch"},
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
					Verbs: []string{"get", "list", "watch"},
				},
			},
		},
		{
			GroupVersion: v1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{
					Group: "v1",
					Name:  "configmaps",
					Kind:  "ConfigMap",
					Verbs: []string{"get", "list", "watch"},
				},
			},
		},
		{
			GroupVersion: policyv1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{
					Group: "policy",
					Name:  "poddisruptionbudgets",
					Kind:  "PodDisruptionBudget",
					Verbs: []string{"get", "list", "watch"},
				},
			},
		},
		{
			GroupVersion: provisionersGvr.GroupVersion().String(),
			APIResources: []metav1.APIResource{
				{
					Group:   provisionersGvr.Group,
					Name:    provisionersGvr.Resource,
					Version: provisionersGvr.Version,
					Kind:    "Provisioner",
					Verbs:   []string{"get", "list", "watch"},
				},
				{
					Group:   machinesGvr.Group,
					Name:    machinesGvr.Resource,
					Version: machinesGvr.Version,
					Kind:    "Machine",
					Verbs:   []string{"get", "list", "watch"},
				},
			},
		},
		{
			GroupVersion: awsNodeTemplatesGvr.GroupVersion().String(),
			APIResources: []metav1.APIResource{
				{
					Group:   awsNodeTemplatesGvr.Group,
					Name:    awsNodeTemplatesGvr.Resource,
					Version: awsNodeTemplatesGvr.Version,
					Kind:    "AWSNodeTemplate",
					Verbs:   []string{"get", "list", "watch"},
				},
			},
		},
		{
			GroupVersion: nodePoolsGvr.GroupVersion().String(),
			APIResources: []metav1.APIResource{
				{
					Group:   nodePoolsGvr.Group,
					Name:    nodePoolsGvr.Resource,
					Version: nodePoolsGvr.Version,
					Kind:    "NodePool",
					Verbs:   []string{"get", "list", "watch"},
				},
				{
					Group:   nodeClaimsGvr.Group,
					Name:    nodeClaimsGvr.Resource,
					Version: nodeClaimsGvr.Version,
					Kind:    "NodeClaim",
					Verbs:   []string{"get", "list", "watch"},
				},
			},
		},
		{
			GroupVersion: ec2NodeClassesGvr.GroupVersion().String(),
			APIResources: []metav1.APIResource{
				{
					Group:   ec2NodeClassesGvr.Group,
					Name:    ec2NodeClassesGvr.Resource,
					Version: ec2NodeClassesGvr.Version,
					Kind:    "EC2NodeClass",
					Verbs:   []string{"get", "list", "watch"},
				},
			},
		},
		{
			GroupVersion: datadogExtendedDSReplicaSetsGvr.GroupVersion().String(),
			APIResources: []metav1.APIResource{
				{
					Group:   datadogExtendedDSReplicaSetsGvr.Group,
					Name:    datadogExtendedDSReplicaSetsGvr.Resource,
					Version: datadogExtendedDSReplicaSetsGvr.Version,
					Kind:    "ExtendedDaemonSetReplicaSet",
					Verbs:   []string{"get", "list", "watch"},
				},
			},
		},
		{
			GroupVersion: argorollouts.RolloutGVR.GroupVersion().String(),
			APIResources: []metav1.APIResource{
				{
					Group:   argorollouts.RolloutGVR.Group,
					Name:    argorollouts.RolloutGVR.Resource,
					Version: argorollouts.RolloutGVR.Version,
					Kind:    "Rollout",
					Verbs:   []string{"get", "list", "watch"},
				},
			},
		},
		{
			GroupVersion: crd.RecommendationGVR.GroupVersion().String(),
			APIResources: []metav1.APIResource{
				{
					Group:   crd.RecommendationGVR.Group,
					Name:    crd.RecommendationGVR.Resource,
					Version: crd.RecommendationGVR.Version,
					Kind:    "Recommendation",
					Verbs:   []string{"get", "list", "watch"},
				},
			},
		},
		{
			GroupVersion: networkingv1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{
					Group: "v1",
					Name:  "ingress",
					Kind:  "Ingress",
					Verbs: []string{"get", "list", "watch"},
				},
				{
					Group: "v1",
					Name:  "networkpolicies",
					Kind:  "NetworkPolicy",
					Verbs: []string{"get", "list", "watch"},
				},
			},
		},
		{
			GroupVersion: rbacv1.SchemeGroupVersion.String(),
			APIResources: []metav1.APIResource{
				{
					Group: "v1",
					Name:  "roles",
					Kind:  "Role",
					Verbs: []string{"get", "list", "watch"},
				},
				{
					Group: "v1",
					Name:  "rolebindings",
					Kind:  "RoleBinding",
					Verbs: []string{"get", "list", "watch"},
				},
				{
					Group: "v1",
					Name:  "clusterroles",
					Kind:  "ClusterRole",
					Verbs: []string{"get", "list", "watch"},
				},
				{
					Group: "v1",
					Name:  "clusterrolebindings",
					Kind:  "ClusterRoleBinding",
					Verbs: []string{"get", "list", "watch"},
				},
			},
		},
	}
	objects := []sampleObject{
		{
			GV:       v1.SchemeGroupVersion,
			Kind:     "Node",
			Resource: "nodes",
			Data:     nodeData,
		},
		{
			GV:       v1.SchemeGroupVersion,
			Kind:     "Pod",
			Resource: "pods",
			Data:     podData,
		},
		{
			GV:       v1.SchemeGroupVersion,
			Kind:     "ConfigMap",
			Resource: "configmaps",
			Data:     cfgMapData,
		},
		{
			GV:       policyv1.SchemeGroupVersion,
			Kind:     "PodDisruptionBudget",
			Resource: "poddisruptionbudgets",
			Data:     pdbData,
		},
		{
			GV:       autoscalingv1.SchemeGroupVersion,
			Kind:     "HorizontalPodAutoscaler",
			Resource: "horizontalpodautoscalers",
			Data:     hpaData,
		},
		{
			GV:       storagev1.SchemeGroupVersion,
			Kind:     "CSINode",
			Resource: "csinodes",
			Data:     csiData,
		},
		{
			GV:       provisionersGvr.GroupVersion(),
			Kind:     "Provisioner",
			Resource: provisionersGvr.Resource,
			Data:     provisionersData,
		},
		{
			GV:       machinesGvr.GroupVersion(),
			Kind:     "Machine",
			Resource: machinesGvr.Resource,
			Data:     machinesData,
		},
		{
			GV:       awsNodeTemplatesGvr.GroupVersion(),
			Kind:     "AWSNodeTemplate",
			Resource: awsNodeTemplatesGvr.Resource,
			Data:     awsNodeTemplatesData,
		},
		{
			GV:       nodePoolsGvr.GroupVersion(),
			Kind:     "NodePool",
			Resource: nodePoolsGvr.Resource,
			Data:     nodePoolsData,
		},
		{
			GV:       nodeClaimsGvr.GroupVersion(),
			Kind:     "NodeClaim",
			Resource: nodeClaimsGvr.Resource,
			Data:     nodeClaimsData,
		},
		{
			GV:       ec2NodeClassesGvr.GroupVersion(),
			Kind:     "EC2NodeClass",
			Resource: ec2NodeClassesGvr.Resource,
			Data:     ec2NodeClassesData,
		},
		{
			GV:       datadogExtendedDSReplicaSetsGvr.GroupVersion(),
			Kind:     "ExtendedDaemonSetReplicaSet",
			Resource: datadogExtendedDSReplicaSetsGvr.Resource,
			Data:     datadogExtendedDSReplicaSetData,
		},
		{
			GV:       argorollouts.RolloutGVR.GroupVersion(),
			Kind:     "Rollout",
			Resource: argorollouts.RolloutGVR.Resource,
			Data:     rolloutData,
		},
		{
			GV:       crd.RecommendationGVR.GroupVersion(),
			Kind:     "Recommendation",
			Resource: crd.RecommendationGVR.Resource,
			Data:     recommendationData,
		},
		{
			GV:       networkingv1.SchemeGroupVersion,
			Kind:     "Ingress",
			Resource: "ingresses",
			Data:     ingressData,
		},
		{
			GV:       networkingv1.SchemeGroupVersion,
			Kind:     "NetworkPolicy",
			Resource: "networkpolicies",
			Data:     netpolicyData,
		},
		{
			GV:       rbacv1.SchemeGroupVersion,
			Kind:     "Role",
			Resource: "roles",
			Data:     roleData,
		},
		{
			GV:       rbacv1.SchemeGroupVersion,
			Kind:     "RoleBinding",
			Resource: "rolebindings",
			Data:     roleBindingData,
		},
		{
			GV:       rbacv1.SchemeGroupVersion,
			Kind:     "ClusterRole",
			Resource: "clusterroles",
			Data:     clusterRoleData,
		},
		{
			GV:       rbacv1.SchemeGroupVersion,
			Kind:     "ClusterRoleBinding",
			Resource: "clusterrolebindings",
			Data:     clusterRoleBindingData,
		},
	}
	// There are a lot of manually entered samples. Running some sanity checks to ensure they don't contain basic errors.
	verifySampleObjectsAreValid(t, objects)

	return objects, clientset, dynamicClient
}

func TestDefaultInformers_MatchFilters(t *testing.T) {
	tests := map[string]struct {
		obj           runtime.Object
		expectedMatch bool
	}{
		"keep if replicaset in castware namespace": {
			obj: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "castware",
				},
			},
			expectedMatch: true,
		},
		"discard if replicaset has zero replicas": {
			obj: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
				},
				Spec: appsv1.ReplicaSetSpec{
					Replicas: lo.ToPtr(int32(0)),
				},
				Status: appsv1.ReplicaSetStatus{
					Replicas: 0,
				},
			},
			expectedMatch: false,
		},
		"keep if replicaset has more than zero replicas": {
			obj: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
				},
				Spec: appsv1.ReplicaSetSpec{
					Replicas: lo.ToPtr(int32(1)),
				},
				Status: appsv1.ReplicaSetStatus{
					Replicas: 1,
				},
			},
			expectedMatch: true,
		},
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(data.obj), 0)

			defaultInformers := getDefaultInformers(f, "castware")
			objInformer := defaultInformers[reflect.TypeOf(data.obj)]

			match := objInformer.filters.Apply(castai.EventAdd, data.obj)

			r.Equal(data.expectedMatch, match)
		})
	}
}

func TestCollectSingleSnapshot(t *testing.T) {
	r := require.New(t)

	mockctrl := gomock.NewController(t)
	version := mock_version.NewMockInterface(mockctrl)
	ctx := context.Background()

	version.EXPECT().Full().Return("1.21+")
	ctx = context.WithValue(ctx, "agentVersion", &config.AgentVersion{
		GitCommit: "test",
		GitRef:    "test",
		Version:   "test",
	})

	var objs []runtime.Object
	for i := range 10000 {
		name := fmt.Sprintf("pod-%d", i)
		objs = append(objs, &v1.Pod{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Pod",
				APIVersion: v1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		})
	}

	clientset := fake.NewSimpleClientset(objs...)

	snapshot, err := CollectSingleSnapshot(
		ctx,
		logrus.New(),
		"123",
		clientset,
		dynamic_fake.NewSimpleDynamicClient(runtime.NewScheme()),
		metrics_fake.NewSimpleClientset(),
		&config.Controller{
			PrepTimeout: 10 * time.Second,
		},
		version,
		"",
	)
	r.NoError(err)
	r.NotNil(snapshot)
	r.Len(snapshot.Items, len(objs))

	var pods []*v1.Pod
	for _, item := range snapshot.Items {
		r.Equal("Pod", item.Kind)
		p := &v1.Pod{}
		r.NoError(json.Unmarshal(*item.Data, p))
		pods = append(pods, p)
	}
	r.ElementsMatch(objs, pods)
}

func unstructuredFromJson(t *testing.T, data []byte) *unstructured.Unstructured {
	var out unstructured.Unstructured
	err := json.Unmarshal(data, &out)
	require.NoError(t, err)
	return &out
}

func asJson(t *testing.T, obj interface{}) []byte {
	data, err := json.Marshal(obj)
	require.NoError(t, err)
	return data
}

func verifySampleObjectsAreValid(t *testing.T, objects []sampleObject) {
	for _, obj := range objects {
		require.NotNil(t, obj.Data)

		var data unstructured.Unstructured
		err := json.Unmarshal(obj.Data, &data)
		require.NoError(t, err)

		gvk := data.GroupVersionKind()
		require.False(t, gvk.Empty())
		require.Equal(t, obj.GV.Group, gvk.Group)
		require.Equal(t, obj.GV.Version, gvk.Version)
		require.Equal(t, obj.Kind, gvk.Kind)
	}
}
