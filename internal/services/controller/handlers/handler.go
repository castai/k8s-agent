package handlers

import (
	"reflect"
	"sync"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"castai-agent/internal/castai"
	"castai-agent/internal/services/controller/delta"
	"castai-agent/internal/services/controller/handlers/filters"
	"castai-agent/internal/services/controller/handlers/transformers"
	"castai-agent/internal/services/metrics"
)

type handler struct {
	mutex        sync.Mutex
	log          logrus.FieldLogger
	handledType  reflect.Type
	queue        workqueue.Interface
	filters      filters.Filters
	transformers transformers.Transformers
}
type Handler interface {
	cache.ResourceEventHandler
}

func NewHandler(
	log logrus.FieldLogger,
	queue workqueue.Interface,
	handledType reflect.Type,
	filters filters.Filters,
	transformers transformers.Transformers,
) Handler {
	return &handler{
		log:          log,
		handledType:  handledType,
		queue:        queue,
		filters:      filters,
		transformers: transformers,
	}
}

func (h *handler) OnAdd(obj interface{}, _ bool) {
	h.handle(castai.EventAdd, obj)
}

func (h *handler) OnUpdate(_, obj interface{}) {
	h.handle(castai.EventUpdate, obj)
}

func (h *handler) OnDelete(obj interface{}) {
	h.handle(castai.EventDelete, obj)
}

func (h *handler) handle(e castai.EventType, obj interface{}) {
	// We start informers before scraping the initial state which means we might get called concurrently with the same object.
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.filters != nil && !h.filters.Apply(e, obj) {
		return
	}

	e, obj = h.transformers.Apply(e, obj)

	metrics.WatchReceived.WithLabelValues(reflect.TypeOf(obj).String(), string(e)).Inc()

	if reflect.TypeOf(obj) != h.handledType {
		// Check if obj is of type *unstructured.Unstructured
		if unstrObj, ok := obj.(*unstructured.Unstructured); ok {
			// Create an instance of the target type
			targetObj := reflect.New(h.handledType.Elem()).Interface()

			// Convert from Unstructured to the target type
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstrObj.UnstructuredContent(), targetObj)
			if err != nil {
				h.log.Errorf("error converting from unstructured to %v: %v", h.handledType, err)
				return
			}

			// Assign the converted object back to obj
			obj = targetObj
		} else {
			h.log.Errorf("expected to get %v but got %T", h.handledType, obj)
			return
		}
	}

	h.log.Debugf("generic handler called: %s: %s", e, reflect.TypeOf(obj))

	item := delta.NewItem(e, obj.(delta.Object))
	h.queue.Add(item)
}
