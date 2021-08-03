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
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"castai-agent/internal/castai"
	mock_castai "castai-agent/internal/castai/mock"
	"castai-agent/internal/config"
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
	expectedNode.Labels[labels.FakeSpot] = "true"
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
		SendDelta(gomock.Any(), clusterID.String(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ context.Context, clusterID string, d *castai.Delta) error {
			defer atomic.AddInt64(&invocations, 1)

			require.Equal(t, clusterID, d.ClusterID)
			require.Equal(t, "1.19+", d.ClusterVersion)
			require.True(t, d.FullSnapshot)
			require.Len(t, d.Items, 2)

			var actualValues []string
			for _, item := range d.Items {
				actualValues = append(actualValues, fmt.Sprintf("%s-%s-%s", item.Event, item.Kind, item.Data))
			}

			require.Contains(t, actualValues, fmt.Sprintf("%s-%s-%s", castai.EventAdd, "Node", nodeData))
			require.Contains(t, actualValues, fmt.Sprintf("%s-%s-%s", castai.EventAdd, "Pod", podData))

			return nil
		})

	agentVersion := &config.AgentVersion{Version: "1.2.3"}
	castaiclient.EXPECT().ExchangeAgentTelemetry(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().
		Return(&castai.AgentTelemetryResponse{}, nil).
		Do(func(ctx context.Context, clusterID string, req *castai.AgentTelemetryRequest) {
			require.Equalf(t, "1.2.3", req.AgentVersion, "got request: %+v", req)
		})
	provider.EXPECT().IsSpot(gomock.Any(), node).Return(true, nil)

	f := informers.NewSharedInformerFactory(clientset, 0)
	ctrl := New(logrus.New(), f, castaiclient, provider, clusterID.String(), 15*time.Second, 100*time.Millisecond, version, agentVersion)
	f.Start(ctx.Done())

	go ctrl.Run(ctx)

	wait.Until(func() {
		if atomic.LoadInt64(&invocations) >= 1 {
			cancel()
		}
	}, 10*time.Millisecond, ctx.Done())
}

func TestCleanObj(t *testing.T) {
	tests := []struct {
		name    string
		obj     interface{}
		matcher func(t *testing.T, obj interface{})
	}{
		{
			name: "should clean managed fields",
			obj: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				ManagedFields: []metav1.ManagedFieldsEntry{
					{
						Manager:    "mngr",
						Operation:  "op",
						APIVersion: "v",
						FieldsType: "t",
					},
				},
			}},
			matcher: func(t *testing.T, obj interface{}) {
				pod := obj.(*v1.Pod)
				require.Nil(t, pod.ManagedFields)
			},
		},
		{
			name: "should remove sensitive env vars from pod",
			obj: &v1.Pod{Spec: v1.PodSpec{Containers: []v1.Container{
				{
					Env: []v1.EnvVar{
						{
							Name:  "LOG_LEVEL",
							Value: "5",
						},
						{
							Name:  "PASSWORD",
							Value: "secret",
						},
						{
							Name: "TOKEN",
							ValueFrom: &v1.EnvVarSource{
								SecretKeyRef: &v1.SecretKeySelector{
									LocalObjectReference: v1.LocalObjectReference{
										Name: "secret",
									},
								},
							},
						},
					},
				},
				{
					Env: []v1.EnvVar{
						{
							Name:  "PWD",
							Value: "super_secret",
						},
					},
				},
			}}},
			matcher: func(t *testing.T, obj interface{}) {
				pod := obj.(*v1.Pod)

				require.Equal(t, []v1.EnvVar{
					{
						Name:  "LOG_LEVEL",
						Value: "5",
					},
					{
						Name: "TOKEN",
						ValueFrom: &v1.EnvVarSource{
							SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{
									Name: "secret",
								},
							},
						},
					},
				}, pod.Spec.Containers[0].Env)

				require.Empty(t, pod.Spec.Containers[1].Env)
			},
		},
		{
			name: "should remove sensitive env vars from statefulset",
			obj: &appsv1.StatefulSet{Spec: appsv1.StatefulSetSpec{Template: v1.PodTemplateSpec{Spec: v1.PodSpec{Containers: []v1.Container{
				{
					Env: []v1.EnvVar{
						{
							Name:  "LOG_LEVEL",
							Value: "5",
						},
						{
							Name:  "PASSWORD",
							Value: "secret",
						},
						{
							Name: "TOKEN",
							ValueFrom: &v1.EnvVarSource{
								SecretKeyRef: &v1.SecretKeySelector{
									LocalObjectReference: v1.LocalObjectReference{
										Name: "secret",
									},
								},
							},
						},
					},
				},
				{
					Env: []v1.EnvVar{
						{
							Name:  "PWD",
							Value: "super_secret",
						},
					},
				},
			}}}}},
			matcher: func(t *testing.T, obj interface{}) {
				sts := obj.(*appsv1.StatefulSet)

				require.Equal(t, []v1.EnvVar{
					{
						Name:  "LOG_LEVEL",
						Value: "5",
					},
					{
						Name: "TOKEN",
						ValueFrom: &v1.EnvVarSource{
							SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{
									Name: "secret",
								},
							},
						},
					},
				}, sts.Spec.Template.Spec.Containers[0].Env)

				require.Empty(t, sts.Spec.Template.Spec.Containers[1].Env)
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			cleanObj(test.obj)
			test.matcher(t, test.obj)
		})
	}
}
