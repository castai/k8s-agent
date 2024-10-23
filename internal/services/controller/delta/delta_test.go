package delta

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"castai-agent/internal/castai"
)

func TestDelta(t *testing.T) {
	clusterID := uuid.New().String()
	version := "1.18"
	agentVersion := "1.2.3"

	unrecognisedResource := &FakeUnrecognisedResource{}

	pod1 := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: v1.NamespaceDefault,
			Name:      "a",
		},
	}
	pod1Updated := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: v1.NamespaceDefault,
			Name:      "a",
			Labels: map[string]string{
				"a": "b",
			},
		},
	}
	pod1WithoutKind := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: v1.NamespaceDefault,
			Name:      "a",
		},
	}
	pod1Unstructured := asUnstructuredObject(t, pod1)

	pod2 := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: v1.NamespaceDefault,
			Name:      "b",
		},
	}

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
			name: "look up missing type information",
			items: []*Item{
				NewItem(castai.EventAdd, pod1WithoutKind),
			},
			expected: &castai.Delta{
				ClusterID:      clusterID,
				ClusterVersion: version,
				FullSnapshot:   true,
				Items: []*castai.DeltaItem{
					{
						Event: castai.EventAdd,
						Kind:  "Pod",
						Data:  mustEncode(t, pod1WithoutKind),
					},
				},
			},
		},
		{
			name: "look up missing type information",
			items: []*Item{
				NewItem(castai.EventAdd, pod1Unstructured),
			},
			expected: &castai.Delta{
				ClusterID:      clusterID,
				ClusterVersion: version,
				FullSnapshot:   true,
				Items: []*castai.DeltaItem{
					{
						Event: castai.EventAdd,
						Kind:  "Pod",
						Data:  mustEncode(t, pod1Unstructured),
					},
				},
			},
		},
		{
			name: "debounce: ignore items without kind",
			items: []*Item{
				NewItem(castai.EventAdd, unrecognisedResource),
			},
			expected: &castai.Delta{
				ClusterID:      clusterID,
				ClusterVersion: version,
				FullSnapshot:   true,
				Items:          []*castai.DeltaItem{},
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
			d := New(logrus.New(), clusterID, version, agentVersion)

			for _, item := range test.items {
				d.Add(item)
			}

			got := d.ToCASTAIRequest()

			require.Equal(t, clusterID, got.ClusterID)
			require.Equal(t, version, got.ClusterVersion)
			require.Equal(t, agentVersion, got.AgentVersion)
			require.True(t, got.FullSnapshot)
			require.Equal(t, len(test.expected.Items), len(got.Items))
			for _, expectedItem := range test.expected.Items {
				requireContains(t, got.Items, expectedItem)
			}
		})
	}
}

var _ = (Object)(&FakeUnrecognisedResource{})

type FakeUnrecognisedResource struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

func (*FakeUnrecognisedResource) DeepCopyObject() runtime.Object {
	panic("not implemented")
}

func mustEncode(t *testing.T, obj interface{}) *json.RawMessage {
	data, err := Encode(obj)
	require.NoError(t, err)
	return data
}

func requireContains(t *testing.T, actual []*castai.DeltaItem, expected *castai.DeltaItem) {
	_, found := lo.Find(actual, func(di *castai.DeltaItem) bool {
		return di.Event == expected.Event && di.Kind == expected.Kind && string(*di.Data) == string(*expected.Data)
	})
	require.Truef(t, found, "expected %s to contain %s", actual, expected)
}

func asUnstructuredObject(t *testing.T, obj runtime.Object) *unstructured.Unstructured {
	bytes, err := json.Marshal(obj)
	require.NoError(t, err)
	var out unstructured.Unstructured
	err = json.Unmarshal(bytes, &out)
	return &out
}
