package util

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

const (
	KindPod           = "Pod"
	ReasonOOMEviction = "Evicted"

	AnnotationStarvedResource = "starved_resource"

	ResourceMemory = "memory"
)

func IsOOMEvent(event *corev1.Event) bool {
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
