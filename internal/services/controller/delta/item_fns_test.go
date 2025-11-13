package delta_test

import (
	"encoding/json"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"castai-agent/internal/services/controller/delta"
	"castai-agent/pkg/castai"
)

func TestItemCacheKey(t *testing.T) {
	type UnknownObject struct {
		corev1.Pod
	}

	testCases := map[string]struct {
		Item *delta.Item
		Key  string
		Err  bool
	}{
		"key for a Pod": {
			Item: &delta.Item{
				Obj: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace-1",
						Name:      "pod-1",
					},
				},
			},
			Key: "/v1, Kind=Pod::namespace-1/pod-1",
		},
		"key for a HPA autoscaling/v1": {
			Item: &delta.Item{
				Obj: &autoscalingv1.HorizontalPodAutoscaler{
					TypeMeta: metav1.TypeMeta{
						Kind:       "HorizontalPodAutoscaler",
						APIVersion: autoscalingv1.SchemeGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace-1",
						Name:      "hpa-1",
					},
				},
			},
			Key: "autoscaling/v1, Kind=HorizontalPodAutoscaler::namespace-1/hpa-1",
		},
		"key for a HPA autoscaling/v2 is not same as autoscaling/v1": {
			Item: &delta.Item{
				Obj: &autoscalingv2.HorizontalPodAutoscaler{
					TypeMeta: metav1.TypeMeta{
						Kind:       "HorizontalPodAutoscaler",
						APIVersion: autoscalingv2.SchemeGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace-1",
						Name:      "hpa-1",
					},
				},
			},
			Key: "autoscaling/v2, Kind=HorizontalPodAutoscaler::namespace-1/hpa-1",
		},
		"key for a Node": {
			Item: &delta.Item{
				Obj: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
					},
				},
			},
			Key: "/v1, Kind=Node::/node-1",
		},
		"prefer kind from TypeMeta": {
			Item: &delta.Item{
				Obj: &corev1.Pod{
					TypeMeta: metav1.TypeMeta{
						Kind: "Custom",
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "pod-1",
					},
				},
			},
			Key: "/, Kind=Custom::default/pod-1",
		},
		"key for unknown type with TypeMeta": {
			Item: &delta.Item{
				Obj: &UnknownObject{
					Pod: corev1.Pod{
						TypeMeta: metav1.TypeMeta{
							Kind: "Custom",
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "pod-1",
						},
					},
				},
			},
			Key: "/, Kind=Custom::default/pod-1",
		},
		"error on unknown type without type information": {
			Item: &delta.Item{
				Obj: &UnknownObject{
					Pod: corev1.Pod{
						TypeMeta: metav1.TypeMeta{
							Kind: "", // No type information stored on the object.
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "pod-1",
						},
					},
				},
			},
			Err: true,
		},
	}

	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			got, err := delta.ItemCacheKey(tt.Item)
			if tt.Err {
				require.Error(t, err)
				require.Empty(t, got)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.Key, got)
			}
		})
	}
}

func TestItemCacheCombine(t *testing.T) {
	pod1 := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: corev1.NamespaceDefault,
			Name:      "a",
		},
	}
	pod1Updated := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: corev1.NamespaceDefault,
			Name:      "a",
			Labels: map[string]string{
				"a": "b",
			},
		},
	}

	testCases := map[string]struct {
		Prev         *delta.Item
		Curr         *delta.Item
		ExpectedType castai.EventType
	}{
		"override added item with updated data": {
			Prev:         delta.NewItem(castai.EventAdd, pod1),
			Curr:         delta.NewItem(castai.EventUpdate, pod1Updated),
			ExpectedType: castai.EventAdd,
		},
		"keep only delete event when an added item is deleted": {
			Prev:         delta.NewItem(castai.EventAdd, pod1),
			Curr:         delta.NewItem(castai.EventDelete, pod1),
			ExpectedType: castai.EventDelete,
		},
		"keep only delete event when an updated item is deleted": {
			Prev:         delta.NewItem(castai.EventUpdate, pod1),
			Curr:         delta.NewItem(castai.EventDelete, pod1),
			ExpectedType: castai.EventDelete,
		},
		"override updated item with newer updated data": {
			Prev:         delta.NewItem(castai.EventUpdate, pod1),
			Curr:         delta.NewItem(castai.EventUpdate, pod1Updated),
			ExpectedType: castai.EventUpdate,
		},
		"change deleted item to updated when it is re-added": {
			Prev:         delta.NewItem(castai.EventDelete, pod1),
			Curr:         delta.NewItem(castai.EventUpdate, pod1Updated),
			ExpectedType: castai.EventUpdate,
		},
	}

	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			got := delta.ItemCacheCombine(tt.Prev, tt.Curr)
			// Input might have been modified, but we always expect latest object to be returned.
			require.Equal(t, tt.Curr, got)
			require.Equal(t, tt.ExpectedType, got.Event)
		})
	}
}

func TestItemCacheCompile(t *testing.T) {
	pod1 := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomType",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: corev1.NamespaceDefault,
			Name:      "a",
		},
	}

	testCases := map[string]struct {
		Item     *delta.Item
		Expected *castai.DeltaItem
	}{
		"compiles item": {
			Item: delta.NewItem(castai.EventAdd, pod1),
			Expected: &castai.DeltaItem{
				Event: castai.EventAdd,
				Data:  lo.ToPtr(json.RawMessage(`{"kind":"CustomType","apiVersion":"v1","metadata":{"name":"a","namespace":"default","creationTimestamp":null},"spec":{"containers":null},"status":{}}`)),
				Kind:  "CustomType",
			},
		},
	}

	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			// Legacy behaviour that should be removed at some point. Item kind is resolved as part of item cache key computation.
			_, _ = delta.ItemCacheKey(tt.Item)

			got, err := delta.ItemCacheCompile(tt.Item)
			require.NoError(t, err)

			require.Equal(t, string(*tt.Expected.Data), string(*got.Data)) // Comparing JSONs separately to produce more user-friendly failure message.
			require.Equal(t, tt.Expected, got)
		})
	}
}
