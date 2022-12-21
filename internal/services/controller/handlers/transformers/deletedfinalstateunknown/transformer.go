package deletedfinalstateunknown

import (
	"k8s.io/client-go/tools/cache"

	"castai-agent/internal/castai"
)

func Transformer(e castai.EventType, obj interface{}) (castai.EventType, interface{}) {
	if obj, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		return castai.EventDelete, obj.Obj
	}

	return e, obj
}
