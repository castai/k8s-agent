package handlers

import (
	"reflect"
	"regexp"

	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"castai-agent/internal/castai"
	"castai-agent/internal/services/controller/delta"
)

var (
	// sensitiveValuePattern matches strings which are usually used to name variables holding sensitive values, like
	// passwords. This is a case-insensitive match of the listed words.
	sensitiveValuePattern = regexp.MustCompile(`(?i)passwd|pass|password|pwd|secret|token|key`)
)

type handler struct {
	log          logrus.FieldLogger
	handledType  reflect.Type
	queue        workqueue.Interface
	filters      Filters
	transformers Transformers
}
type Handler interface {
	cache.ResourceEventHandler
}

func NewHandler(log logrus.FieldLogger, queue workqueue.Interface, handledType reflect.Type) Handler {
	return &handler{
		log:         log,
		handledType: handledType,
		queue:       queue,
		filters:     Filters{},
		transformers: Transformers{
			NewDeletedFinalStateUnknownTransformer(),
		},
	}
}

func (h *handler) handle(e castai.EventType, obj interface{}) {
	if !h.filters.apply(e, obj) {
		return
	}

	e, obj = h.transformers.apply(e, obj)

	if reflect.TypeOf(obj) != h.handledType {
		h.log.Errorf("expected to get %v but got %T", h.handledType, obj)
		return
	}

	cleanObj(obj)

	h.log.Debugf("generic handler called: %s: %s", e, reflect.TypeOf(obj))

	item := delta.NewItem(e, obj.(delta.Object))
	h.queue.Add(item)
}

// cleanObj removes unnecessary or sensitive values from K8s objects.
func cleanObj(obj interface{}) {
	removeManagedFields(obj)
	removeSensitiveEnvVars(obj)
}

func removeManagedFields(obj interface{}) {
	if metaobj, ok := obj.(metav1.Object); ok {
		metaobj.SetManagedFields(nil)
	}
}

func removeSensitiveEnvVars(obj interface{}) {
	var containers []*corev1.Container

	switch o := obj.(type) {
	case *corev1.Pod:
		for i := range o.Spec.Containers {
			containers = append(containers, &o.Spec.Containers[i])
		}
	case *appsv1.Deployment:
		for i := range o.Spec.Template.Spec.Containers {
			containers = append(containers, &o.Spec.Template.Spec.Containers[i])
		}
	case *appsv1.StatefulSet:
		for i := range o.Spec.Template.Spec.Containers {
			containers = append(containers, &o.Spec.Template.Spec.Containers[i])
		}
	case *appsv1.ReplicaSet:
		for i := range o.Spec.Template.Spec.Containers {
			containers = append(containers, &o.Spec.Template.Spec.Containers[i])
		}
	case *appsv1.DaemonSet:
		for i := range o.Spec.Template.Spec.Containers {
			containers = append(containers, &o.Spec.Template.Spec.Containers[i])
		}
	}

	if len(containers) == 0 {
		return
	}

	for _, c := range containers {
		c.Env = lo.Filter(c.Env, func(envVar corev1.EnvVar, _ int) bool {
			return envVar.Value == "" || !sensitiveValuePattern.MatchString(envVar.Name)
		})
	}
}

func (h *handler) OnAdd(obj interface{}) {
	h.handle(castai.EventAdd, obj)
}

func (h *handler) OnUpdate(_, obj interface{}) {
	h.handle(castai.EventUpdate, obj)
}

func (h *handler) OnDelete(obj interface{}) {
	h.handle(castai.EventDelete, obj)
}
