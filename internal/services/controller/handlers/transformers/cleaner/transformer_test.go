package cleaner

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"castai-agent/internal/castai"
)

func Test_Transformer(t *testing.T) {
	tests := []struct {
		name    string
		obj     interface{}
		matcher func(r *require.Assertions, obj interface{})
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
			matcher: func(r *require.Assertions, obj interface{}) {
				pod := obj.(*v1.Pod)
				r.Nil(pod.ManagedFields)
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
						{
							Name:  "GCP_CREDENTIALS",
							Value: "super secret",
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
				{
					Env: []v1.EnvVar{
						{
							Name:  "API_KEY",
							Value: "secret",
						},
						{
							Name:  "TIMEOUT",
							Value: "1s",
						},
					},
				},
			}}},
			matcher: func(r *require.Assertions, obj interface{}) {
				pod := obj.(*v1.Pod)

				r.Equal([]v1.EnvVar{
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

				r.Empty(pod.Spec.Containers[1].Env)

				r.Equal([]v1.EnvVar{
					{
						Name:  "TIMEOUT",
						Value: "1s",
					},
				}, pod.Spec.Containers[2].Env)

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
			matcher: func(r *require.Assertions, obj interface{}) {
				sts := obj.(*appsv1.StatefulSet)

				r.Equal([]v1.EnvVar{
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

				r.Empty(sts.Spec.Template.Spec.Containers[1].Env)
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			r := require.New(t)

			for _, event := range []castai.EventType{castai.EventAdd, castai.EventUpdate, castai.EventDelete} {
				Transformer(event, test.obj)
				test.matcher(r, test.obj)
			}
		})
	}
}
