package autoscalerevents

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"castai-agent/internal/castai"
)

const (
	AutoscalerController = "autoscaler.cast.ai"
	fieldSelector        = "reportingComponent=" + AutoscalerController
)

func Filter(_ castai.EventType, obj interface{}) bool {
	event, ok := obj.(*corev1.Event)
	if !ok {
		return false
	}
	return event.ReportingController == AutoscalerController
}

func ListOpts(opts *metav1.ListOptions) {
	opts.FieldSelector = fieldSelector
}
