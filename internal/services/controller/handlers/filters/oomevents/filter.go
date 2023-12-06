package oomevents

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"castai-agent/internal/castai"
	"castai-agent/internal/services/version"
)

const (
	KindPod           = "Pod"
	ReasonOOMEviction = "Evicted"

	AnnotationStarvedResource = "starved_resource"

	ResourceMemory = "memory"

	fieldSelector = "involvedObject.kind=" + KindPod + ",reason=" + ReasonOOMEviction
)

func Filter(_ castai.EventType, obj interface{}) bool {
	event, ok := obj.(*corev1.Event)
	if !ok {
		return false
	}

	if event.Reason != ReasonOOMEviction {
		return false
	}

	if event.InvolvedObject.Kind != KindPod {
		return false
	}

	if event.Annotations == nil {
		return false
	}

	// starvedResourcesString contains a list of starved resources separated by commas.
	starvedResourcesString, starvedResourcesFound := event.Annotations[AnnotationStarvedResource]
	if !starvedResourcesFound {
		return false
	}

	return strings.Contains(starvedResourcesString, ResourceMemory)
}

func ListOpts(opts *metav1.ListOptions, _ version.Interface) {
	opts.FieldSelector = fieldSelector
}
