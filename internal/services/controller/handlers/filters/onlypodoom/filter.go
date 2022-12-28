package onlypodoom

import (
	corev1 "k8s.io/api/core/v1"

	"castai-agent/internal/castai"
	"castai-agent/internal/services/controller/util"
)

func Filter(e castai.EventType, obj interface{}) bool {
	k8sEvent, ok := obj.(*corev1.Event)
	if !ok {
		return false
	}

	return util.IsOOMEvent(k8sEvent)
}
