package autoscalerevents

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sversion "k8s.io/apimachinery/pkg/util/version"

	"castai-agent/internal/castai"
	"castai-agent/internal/services/version"
)

const (
	AutoscalerController = "autoscaler.cast.ai"
	fieldSelector        = "reportingComponent=" + AutoscalerController
)

var minFieldSelectorVersion = k8sversion.MustParseSemantic("1.19.0")

func Filter(_ castai.EventType, obj interface{}) bool {
	event, ok := obj.(*corev1.Event)
	if !ok {
		return false
	}
	return event.ReportingController == AutoscalerController
}

func ListOpts(opts *metav1.ListOptions, clusterVersion version.Interface) {
	ver, err := k8sversion.ParseSemantic(clusterVersion.Full())
	if err == nil && ver.AtLeast(minFieldSelectorVersion) {
		opts.FieldSelector = fieldSelector
	}
}
