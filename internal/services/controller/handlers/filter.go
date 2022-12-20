package handlers

import (
	corev1 "k8s.io/api/core/v1"

	"castai-agent/internal/castai"
	"castai-agent/internal/services/controller/util"
)

type Filter func(e castai.EventType, obj interface{}) bool
type Filters []Filter

func (fs Filters) apply(e castai.EventType, obj interface{}) bool {
	for _, f := range fs {
		f := f
		if !f(e, obj) {
			return false
		}
	}

	return true
}

func NewOnlyPodOOMEventFilter() Filter {
	return func(e castai.EventType, obj interface{}) bool {
		//TODO: check if the type is a pointer type
		k8sEvent, ok := obj.(*corev1.Event)
		if !ok {
			return false
		}

		return util.IsOOMEvent(k8sEvent)
	}
}
