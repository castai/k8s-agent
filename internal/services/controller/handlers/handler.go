package handlers

import (
	"castai-agent/internal/services/controller/handlers/transformers"
	"castai-agent/internal/services/controller/handlers/transformers/cleaner"
	"castai-agent/internal/services/controller/handlers/transformers/deletedfinalstateunknown"
	"reflect"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"castai-agent/internal/castai"
	"castai-agent/internal/services/controller/delta"
)

type handler struct {
	log          logrus.FieldLogger
	handledType  reflect.Type
	queue        workqueue.Interface
	filters      Filters
	transformers transformers.Transformers
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
		transformers: transformers.Transformers{
			deletedfinalstateunknown.New(),
			cleaner.New(),
		},
	}
}

func (h *handler) handle(e castai.EventType, obj interface{}) {
	if !h.filters.apply(e, obj) {
		return
	}

	e, obj = h.transformers.Apply(e, obj)

	if reflect.TypeOf(obj) != h.handledType {
		h.log.Errorf("expected to get %v but got %T", h.handledType, obj)
		return
	}

	h.log.Debugf("generic handler called: %s: %s", e, reflect.TypeOf(obj))

	item := delta.NewItem(e, obj.(delta.Object))
	h.queue.Add(item)
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
