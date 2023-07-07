package autoscalerevents

import (
	corev1 "k8s.io/api/core/v1"

	"castai-agent/internal/castai"
)

const (
	AutoscalerController = "autoscaler.cast.ai"
)

func Filter(_ castai.EventType, obj interface{}) bool {
	event, ok := obj.(*corev1.Event)
	if !ok {
		return false
	}
	return event.ReportingController == AutoscalerController
}
