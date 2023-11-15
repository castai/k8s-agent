package handlers

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/sirupsen/logrus"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"castai-agent/internal/castai"
	"castai-agent/internal/services/controller/delta"
	"castai-agent/internal/services/controller/handlers/transformers"
	mock_workqueue "castai-agent/internal/services/controller/mock"
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

	items := []delta.Object{pod, node, pv, pvc, rc, ns, service, hpa, pdb, cfgmap}

	for _, item := range items {
		item := item
		t.Run(fmt.Sprintf("should handle all events for object type %v", item.GetName()), func(t *testing.T) {
			queue := mock_workqueue.NewMockInterface(gomock.NewController(t))

			h := NewHandler(logrus.New(), queue, reflect.TypeOf(item), nil, transformers.Transformers{})
			queue.EXPECT().Add(delta.NewItem(castai.EventAdd, item))
			h.OnAdd(item, true)

			queue.EXPECT().Add(delta.NewItem(castai.EventUpdate, item))
			h.OnUpdate(item, item)

			queue.EXPECT().Add(delta.NewItem(castai.EventDelete, item))
			h.OnDelete(item)
		})
	}
}
