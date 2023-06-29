package oomevents

import (
	"castai-agent/internal/castai"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestFilter(t *testing.T) {
	tt := map[string]struct {
		event *corev1.Event
		want  bool
	}{
		"returns true if the object is of type *Event and has the ReportingController field set to OOMEvent": {
			event: &corev1.Event{
				InvolvedObject: corev1.ObjectReference{
					Kind: KindPod,
				},
				Reason: ReasonOOMEviction,
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationStarvedResource: ResourceMemory,
					},
				},
			},
			want: true,
		},
		"returns false if the object is of type *Event but does not have the ReportingController field set to OOMEvent": {
			event: &corev1.Event{
				Reason: "something else",
			},
			want: false,
		},
	}
	for name, tc := range tt {
		t.Run(name, func(t *testing.T) {
			filter := Filter(castai.EventAdd, tc.event)
			require.Equal(t, tc.want, filter)
		})
	}
}
