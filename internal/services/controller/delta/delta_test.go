package delta

import (
	"encoding/json"
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
		items    []*Item
		expected *castai.Delta
	}{
		{
			name:  "empty items",
			items: []*Item{},
			expected: &castai.Delta{
				ClusterID:      clusterID,
				ClusterVersion: version,
				FullSnapshot:   true,
			},
		},
		{
			name: "multiple items",
			items: []*Item{
				NewItem(castai.EventAdd, pod1),
				NewItem(castai.EventAdd, pod2),
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
			items: []*Item{
				NewItem(castai.EventAdd, pod1),
				NewItem(castai.EventUpdate, pod1Updated),
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
			items: []*Item{
				NewItem(castai.EventAdd, pod1),
				NewItem(castai.EventDelete, pod1),
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
			items: []*Item{
				NewItem(castai.EventUpdate, pod1),
				NewItem(castai.EventDelete, pod1),
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
			items: []*Item{
				NewItem(castai.EventUpdate, pod1),
				NewItem(castai.EventUpdate, pod1Updated),
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
			name: "debounce: change deleted item to updated when it is re-added",
			items: []*Item{
				NewItem(castai.EventDelete, pod1),
				NewItem(castai.EventUpdate, pod1Updated),
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
			d := New(logrus.New(), clusterID, version)

			for _, item := range test.items {
				d.Add(item)
			}

			got := d.ToCASTAIRequest()

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

func mustEncode(t *testing.T, obj interface{}) *json.RawMessage {
	data, err := Encode(obj)
	require.NoError(t, err)
	return data
}

func requireContains(t *testing.T, actual []*castai.DeltaItem, expected *castai.DeltaItem) {
	for _, di := range actual {
		if di.Kind == expected.Kind && di.Event == expected.Event && string(*di.Data) == string(*expected.Data) {
			return
		}
	}
	require.Failf(t, "failed", "expected %s to contain %s", actual, expected)
}
