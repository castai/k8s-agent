package informers

import (
	"reflect"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"castai-agent/internal/services/controller/handlers"
	"castai-agent/internal/services/controller/handlers/filters"
	"castai-agent/internal/services/controller/handlers/transformers"
	"castai-agent/internal/services/controller/handlers/transformers/cleaner"
	"castai-agent/internal/services/controller/handlers/transformers/deletedfinalstateunknown"
)

type HandledInformer struct {
	cache.SharedInformer
	Handler handlers.Handler
}

var defaultTransformers = transformers.Transformers{
	deletedfinalstateunknown.Transformer,
	cleaner.Transformer,
}

func NewHandledInformer(
	log logrus.FieldLogger,
	queue workqueue.Interface,
	informer cache.SharedInformer,
	handledType reflect.Type,
	filters filters.Filters,
	additionalTransformers ...transformers.Transformer,
) *HandledInformer {
	log = log.WithField("informer", handledType.String())
	transformers := append(defaultTransformers, additionalTransformers...)
	handler := handlers.NewHandler(log, queue, handledType, filters, transformers)
	informer.AddEventHandler(handler)

	return &HandledInformer{
		SharedInformer: informer,
		Handler:        handler,
	}
}
