package controller

import (
	"testing"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"castai-agent/internal/castai"
)

func TestDelta(t *testing.T) {
	clusterID := uuid.New().String()
	version := "1.18"

	pod1 := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: v1.NamespaceDefault, Name: "a"}}
	pod1Updated := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: v1.NamespaceDefault, Name: "a", Labels: map[string]string{"a": "b"}}}

	pod2 := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: v1.NamespaceDefault, Name: "b"}}

	tests := []struct {
		name     string
		items    []*item
		expected *castai.Delta
	}{
		{
			name:  "empty items",
			items: []*item{},
			expected: &castai.Delta{
				ClusterID:      clusterID,
				ClusterVersion: version,
				FullSnapshot:   true,
			},
		},
		{
			name: "multiple items",
			items: []*item{
				{
					obj:   pod1,
					event: eventAdd,
				},
				{
					obj:   pod2,
					event: eventAdd,
				},
			},
			expected: &castai.Delta{
				ClusterID:      clusterID,
				ClusterVersion: version,
				FullSnapshot:   true,
				Items: []*castai.DeltaItem{
					{
						Event: castai.EventAdd,
						Kind:  "Pod",
						Data:  mustEncode(t, pod1),
					},
					{
						Event: castai.EventAdd,
						Kind:  "Pod",
						Data:  mustEncode(t, pod2),
					},
				},
			},
		},
		{
			name: "debounce: override added item with updated data",
			items: []*item{
				{
					obj:   pod1,
					event: eventAdd,
				},
				{
					obj:   pod1Updated,
					event: eventUpdate,
				},
			},
			expected: &castai.Delta{
				ClusterID:      clusterID,
				ClusterVersion: version,
				FullSnapshot:   true,
				Items: []*castai.DeltaItem{
					{
						Event: castai.EventAdd,
						Kind:  "Pod",
						Data:  mustEncode(t, pod1Updated),
					},
				},
			},
		},
		{
			name: "debounce: keep only delete event when an added item is deleted",
			items: []*item{
				{
					obj:   pod1,
					event: eventAdd,
				},
				{
					obj:   pod1,
					event: eventDelete,
				},
			},
			expected: &castai.Delta{
				ClusterID:      clusterID,
				ClusterVersion: version,
				FullSnapshot:   true,
				Items: []*castai.DeltaItem{
					{
						Event: castai.EventDelete,
						Kind:  "Pod",
						Data:  mustEncode(t, pod1),
					},
				},
			},
		},
		{
			name: "debounce: keep only delete event when an updated item is deleted",
			items: []*item{
				{
					obj:   pod1,
					event: eventUpdate,
				},
				{
					obj:   pod1,
					event: eventDelete,
				},
			},
			expected: &castai.Delta{
				ClusterID:      clusterID,
				ClusterVersion: version,
				FullSnapshot:   true,
				Items: []*castai.DeltaItem{
					{
						Event: castai.EventDelete,
						Kind:  "Pod",
						Data:  mustEncode(t, pod1),
					},
				},
			},
		},
		{
			name: "debounce: override updated item with newer updated data",
			items: []*item{
				{
					obj:   pod1,
					event: eventUpdate,
				},
				{
					obj:   pod1Updated,
					event: eventUpdate,
				},
			},
			expected: &castai.Delta{
				ClusterID:      clusterID,
				ClusterVersion: version,
				FullSnapshot:   true,
				Items: []*castai.DeltaItem{
					{
						Event: castai.EventUpdate,
						Kind:  "Pod",
						Data:  mustEncode(t, pod1Updated),
					},
				},
			},
		},
		{
			name: "debounce: change deleted item to updated when it is readded",
			items: []*item{
				{
					obj:   pod1,
					event: eventDelete,
				},
				{
					obj:   pod1Updated,
					event: eventAdd,
				},
			},
			expected: &castai.Delta{
				ClusterID:      clusterID,
				ClusterVersion: version,
				FullSnapshot:   true,
				Items: []*castai.DeltaItem{
					{
						Event: castai.EventUpdate,
						Kind:  "Pod",
						Data:  mustEncode(t, pod1Updated),
					},
				},
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			d := newDelta(logrus.New(), clusterID, version)

			for _, item := range test.items {
				d.add(item)
			}

			got := d.toCASTAIRequest()

			require.Equal(t, clusterID, got.ClusterID)
			require.Equal(t, version, got.ClusterVersion)
			require.True(t, got.FullSnapshot)
			require.Equal(t, len(test.expected.Items), len(got.Items))
			for _, expectedItem := range test.expected.Items {
				requireContains(t, got.Items, expectedItem)
			}
		})
	}
}

func mustEncode(t *testing.T, obj interface{}) string {
	data, err := encode(obj)
	require.NoError(t, err)
	return data
}

func requireContains(t *testing.T, actual []*castai.DeltaItem, expected *castai.DeltaItem) {
	for _, di := range actual {
		if di.Kind == expected.Kind && di.Event == expected.Event && di.Data == expected.Data {
			return
		}
	}
	require.Failf(t, "failed", "expected %s to contain %s", actual, expected)
}
