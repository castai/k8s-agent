package handlers

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"castai-agent/internal/castai"
	"castai-agent/internal/services/controller/delta"
	"castai-agent/internal/services/controller/handlers/transformers"

	mock_workqueue "castai-agent/mocks/k8s.io/client-go/util/workqueue"
)

func Test_handler(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod",
			Namespace: v1.NamespaceDefault,
		},
	}

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node",
			Namespace: v1.NamespaceDefault,
		},
	}

	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "persistentvolume",
			Namespace: v1.NamespaceDefault,
		},
	}

	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "persistentvolumeclaim",
			Namespace: v1.NamespaceDefault,
		},
	}

	rc := &v1.ReplicationController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "replicationcontroller",
			Namespace: v1.NamespaceDefault,
		},
	}

	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "namespace",
		},
	}

	service := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "service",
		},
	}

	hpa := &autoscalingv1.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "horizontalpodautoscaler",
			Namespace: v1.NamespaceDefault,
		},
	}

	hpaV2 := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "horizontalpodautoscaler",
			Namespace: v1.NamespaceDefault,
		},
	}
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "poddisruptionbudget",
			Namespace: v1.NamespaceDefault,
		},
	}
	cfgmap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: v1.NamespaceDefault, Name: "cfg1"},
		Data:       map[string]string{"field1": "value1"},
	}

	items := []delta.Object{pod, node, pv, pvc, rc, ns, service, hpa, hpaV2, pdb, cfgmap}

	for _, item := range items {
		t.Run(fmt.Sprintf("should handle all events for object type %v", item.GetName()), func(t *testing.T) {
			queue := mock_workqueue.NewMockInterface(t)

			h := NewHandler(logrus.New(), queue, reflect.TypeOf(item), nil, transformers.Transformers{})
			var queueAddCount int
			queue.EXPECT().
				Add(mock.Anything).
				Run(func(i interface{}) {
					queueAddCount++

					// Assert that the item is of the expected type
					actual, ok := i.(*delta.Item)
					assert.True(t, ok)
					assert.Equal(t, item, actual.Obj)

					switch queueAddCount {
					case 1:

						assert.Equal(t, castai.EventAdd, actual.Event)
					case 2:
						assert.Equal(t, castai.EventUpdate, actual.Event)
					case 3:
						assert.Equal(t, castai.EventDelete, actual.Event)
					}
				})
			h.OnAdd(item, true)
			h.OnUpdate(item, item)
			h.OnDelete(item)
		})
	}
}
