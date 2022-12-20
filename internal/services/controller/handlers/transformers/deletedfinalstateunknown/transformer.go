package deletedfinalstateunknown

import (
	"k8s.io/client-go/tools/cache"

	"castai-agent/internal/castai"
	"castai-agent/internal/services/controller/handlers/transformers"
)

func New() transformers.Transformer {
	return func(e castai.EventType, obj interface{}) (castai.EventType, interface{}) {
		if obj, ok := obj.(cache.DeletedFinalStateUnknown); ok {
			return e, obj
		}

		return e, obj
	}
}
