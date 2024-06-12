package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"castai-agent/internal/services/controller/crd"
	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	argorollouts "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	karpenterCoreAlpha "github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	karpenterCore "github.com/aws/karpenter-core/pkg/apis/v1beta1"
	karpenterAlpha "github.com/aws/karpenter/pkg/apis/v1alpha1"
	karpenter "github.com/aws/karpenter/pkg/apis/v1beta1"
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
	policyv1 "k8s.io/api/policy/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	"castai-agent/internal/services/controller/delta"
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
			expectedReceivedObjectsCount: 15,
		},
		"when fetching api resources produces multiple errors should exclude those resources": {
			apiResourceError: fmt.Errorf("unable to retrieve the complete list of server APIs: %v:"+
				"stale GroupVersion discovery: some error,%v: another error",
				policyv1.SchemeGroupVersion.String(), storagev1.SchemeGroupVersion.String()),
			expectedReceivedObjectsCount: 13,
		},
		"when fetching api resources produces single error should exclude that resource": {
			apiResourceError: fmt.Errorf("unable to retrieve the complete list of server APIs: %v:"+
				"stale GroupVersion discovery: some error", storagev1.SchemeGroupVersion.String()),
			expectedReceivedObjectsCount: 14,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			utilruntime.Must(karpenterCoreAlpha.SchemeBuilder.AddToScheme(scheme))
			utilruntime.Must(karpenterAlpha.SchemeBuilder.AddToScheme(scheme))
			utilruntime.Must(karpenterCore.SchemeBuilder.AddToScheme(scheme))
			utilruntime.Must(karpenter.SchemeBuilder.AddToScheme(scheme))
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
				for k := range objectsData {
					kLowerCased := strings.ToLower(k)
					found := false
					for _, resource := range apiResources {
						if strings.Contains(resource.APIResources[0].Name, kLowerCased) {
							found = true
							break
						}
					}
					if !found {
						delete(objectsData, k)
					}
				}
				mockDiscovery.EXPECT().ServerGroupsAndResources().Return([]*metav1.APIGroup{}, apiResources, tt.apiResourceError).AnyTimes()
			}
			var invocations int64

			castaiclient.EXPECT().
				SendDelta(gomock.Any(), clusterID.String(), gomock.Any()).AnyTimes().
				DoAndReturn(func(_ context.Context, clusterID string, d *castai.Delta) error {
					defer atomic.AddInt64(&invocations, 1)

					require.Equal(t, clusterID, d.ClusterID)
					require.Equal(t, "1.21+", d.ClusterVersion)
					require.True(t, d.FullSnapshot)
					require.Len(t, d.Items, tt.expectedReceivedObjectsCount)

					var actualValues []string
					for _, item := range d.Items {
						actualValues = append(actualValues, fmt.Sprintf("%s-%s-%v", item.Event, item.Kind, item.Data))
					}

					for k, v := range objectsData {
						require.Contains(t, actualValues, fmt.Sprintf("%s-%s-%v", castai.EventAdd, k, v))
					}

					return nil
				})

			agentVersion := &config.AgentVersion{Version: "1.2.3"}
			castaiclient.EXPECT().ExchangeAgentTelemetry(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().
				Return(&castai.AgentTelemetryResponse{}, nil).
				Do(func(ctx context.Context, clusterID string, req *castai.AgentTelemetryRequest) {
					require.Equalf(t, "1.2.3", req.AgentVersion, "got request: %+v", req)
				})

			node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{}}}
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

func TestController_ShouldKeepDeltaAfterDelete(t *testing.T) {
	mockctrl := gomock.NewController(t)
	castaiclient := mock_castai.NewMockClient(mockctrl)
	version := mock_version.NewMockInterface(mockctrl)
	provider := mock_types.NewMockProvider(mockctrl)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: v1.NamespaceDefault, Name: "pod1"}}
	podData, err := delta.Encode(pod)
	require.NoError(t, err)

	clientset := fake.NewSimpleClientset()
	metricsClient := metrics_fake.NewSimpleClientset()
	dynamicClient := dynamic_fake.NewSimpleDynamicClient(runtime.NewScheme())

	version.EXPECT().Full().Return("1.21+").MaxTimes(3)

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
			require.False(t, d.FullSnapshot)
			require.Len(t, d.Items, 1)

			var actualValues []string
			for _, item := range d.Items {
				actualValues = append(actualValues, fmt.Sprintf("%s-%s-%v", item.Event, item.Kind, item.Data))
			}

			require.Contains(t, actualValues, fmt.Sprintf("%s-%s-%v", castai.EventAdd, "Pod", podData))

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
			require.False(t, d.FullSnapshot)
			require.Len(t, d.Items, 1)

			var actualValues []string
			for _, item := range d.Items {
				actualValues = append(actualValues, fmt.Sprintf("%s-%s-%v", item.Event, item.Kind, item.Data))
			}

			require.Contains(t, actualValues, fmt.Sprintf("%s-%s-%v", castai.EventDelete, "Pod", podData))

			return nil
		})

	agentVersion := &config.AgentVersion{Version: "1.2.3"}
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

func loadInitialHappyPathData(t *testing.T, scheme *runtime.Scheme) (map[string]*json.RawMessage, *fake.Clientset, *dynamic_fake.FakeDynamicClient) {
	provisionersGvr := karpenterCoreAlpha.SchemeGroupVersion.WithResource("provisioners")
	machinesGvr := karpenterCoreAlpha.SchemeGroupVersion.WithResource("machines")
	awsNodeTemplatesGvr := karpenterAlpha.SchemeGroupVersion.WithResource("awsnodetemplates")
	nodePoolsGvr := karpenterCore.SchemeGroupVersion.WithResource("nodepools")
	nodeClaimsGvr := karpenterCore.SchemeGroupVersion.WithResource("nodeclaims")
	ec2NodeClassesGvr := karpenter.SchemeGroupVersion.WithResource("ec2nodeclasses")
	datadogExtendedDSReplicaSetsGvr := datadoghqv1alpha1.GroupVersion.WithResource("extendeddaemonsetreplicasets")

	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{}}}
	expectedNode := node.DeepCopy()
	expectedNode.Labels[labels.CastaiFakeSpot] = "true"
	nodeData, err := delta.Encode(expectedNode)
	require.NoError(t, err)

	pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: v1.NamespaceDefault, Name: "pod1"}}
	podData, err := delta.Encode(pod)
	require.NoError(t, err)

	cfgMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: v1.NamespaceDefault, Name: "cfg1"},
		Data:       map[string]string{"field1": "value1"},
	}
	cfgMapData, err := delta.Encode(cfgMap)
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

	provisioners := &karpenterCoreAlpha.Provisioner{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Provisioner",
			APIVersion: provisionersGvr.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      provisionersGvr.Resource,
			Namespace: v1.NamespaceDefault,
		},
	}

	provisionersData, err := delta.Encode(provisioners)
	require.NoError(t, err)

	machines := &karpenterCoreAlpha.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Machine",
			APIVersion: machinesGvr.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      machinesGvr.Resource,
			Namespace: v1.NamespaceDefault,
		},
	}

	machinesData, err := delta.Encode(machines)
	require.NoError(t, err)

	awsNodeTemplates := &karpenterAlpha.AWSNodeTemplate{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AWSNodeTemplate",
			APIVersion: awsNodeTemplatesGvr.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      awsNodeTemplatesGvr.Resource,
			Namespace: v1.NamespaceDefault,
		},
	}

	awsNodeTemplatesData, err := delta.Encode(awsNodeTemplates)
	require.NoError(t, err)

	nodePools := &karpenterCore.NodePool{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NodePool",
			APIVersion: nodePoolsGvr.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodePoolsGvr.Resource,
			Namespace: v1.NamespaceDefault,
		},
	}

	nodePoolsData, err := delta.Encode(nodePools)
	require.NoError(t, err)

	nodeClaims := &karpenterCore.NodeClaim{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NodeClaim",
			APIVersion: nodeClaimsGvr.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodeClaimsGvr.Resource,
			Namespace: v1.NamespaceDefault,
		},
	}

	nodeClaimsData, err := delta.Encode(nodeClaims)
	require.NoError(t, err)

	ec2NodeClasses := &karpenter.EC2NodeClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       "EC2NodeClass",
			APIVersion: ec2NodeClassesGvr.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ec2NodeClassesGvr.Resource,
			Namespace: v1.NamespaceDefault,
		},
	}

	ec2NodeClassesData, err := delta.Encode(ec2NodeClasses)
	require.NoError(t, err)

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

	datadogExtendedDSReplicaSetData, err := delta.Encode(datadogExtendedDSReplicaSet)
	require.NoError(t, err)

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

	rolloutData, err := delta.Encode(rollout)
	require.NoError(t, err)

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

	recommendationData, err := delta.Encode(recommendation)
	require.NoError(t, err)

	clientset := fake.NewSimpleClientset(node, pod, cfgMap, pdb, hpa, csi)
	dynamicClient := dynamic_fake.NewSimpleDynamicClient(scheme, provisioners, machines, awsNodeTemplates, nodePools, nodeClaims, ec2NodeClasses, datadogExtendedDSReplicaSet, rollout, recommendation)
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
					Verbs:   []string{"get", "list", "watch"},
				},
			},
		},
		{
			GroupVersion: machinesGvr.GroupVersion().String(),
			APIResources: []metav1.APIResource{
				{
					Group:   machinesGvr.Group,
					Name:    machinesGvr.Resource,
					Version: machinesGvr.Version,
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
					Verbs:   []string{"get", "list", "watch"},
				},
			},
		},
		{
			GroupVersion: nodeClaimsGvr.GroupVersion().String(),
			APIResources: []metav1.APIResource{
				{
					Group:   nodeClaimsGvr.Group,
					Name:    nodeClaimsGvr.Resource,
					Version: nodeClaimsGvr.Version,
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
					Verbs:   []string{"get", "list", "watch"},
				},
			},
		},
	}
	objects := make(map[string]*json.RawMessage)
	objects["Node"] = nodeData
	objects["Pod"] = podData
	objects["ConfigMap"] = cfgMapData
	objects["PodDisruptionBudget"] = pdbData
	objects["HorizontalPodAutoscaler"] = hpaData
	objects["CSINode"] = csiData
	objects["Provisioner"] = provisionersData
	objects["Machine"] = machinesData
	objects["AWSNodeTemplate"] = awsNodeTemplatesData
	objects["NodePool"] = nodePoolsData
	objects["NodeClaim"] = nodeClaimsData
	objects["EC2NodeClass"] = ec2NodeClassesData
	objects["ExtendedDaemonSetReplicaSet"] = datadogExtendedDSReplicaSetData
	objects["Rollout"] = rolloutData
	objects["Recommendation"] = recommendationData

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
		"keep if deployment in castware namespace": {
			obj: &appsv1.Deployment{
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
		"discard if deployment has zero replicas": {
			obj: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: lo.ToPtr(int32(0)),
				},
				Status: appsv1.DeploymentStatus{
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
		"keep if deployment has more than zero replicas": {
			obj: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: lo.ToPtr(int32(1)),
				},
				Status: appsv1.DeploymentStatus{
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
