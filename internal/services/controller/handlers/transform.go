package handlers

import (
	"k8s.io/client-go/tools/cache"

	"castai-agent/internal/castai"
)

type Transformer func(e castai.EventType, obj interface{}) (castai.EventType, interface{})

type Transformers []Transformer

func (ts Transformers) apply(e castai.EventType, obj interface{}) (castai.EventType, interface{}) {
	for _, t := range ts {
		t := t
		e, obj = t(e, obj)
	}

	return e, obj
}

func NewDeletedFinalStateUnknownTransformer() Transformer {
	return func(e castai.EventType, obj interface{}) (castai.EventType, interface{}) {
		if obj, ok := obj.(cache.DeletedFinalStateUnknown); ok {
			return e, obj
		}

		return e, obj
	}
}
