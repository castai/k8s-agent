package autoscalerevents

import (
	"castai-agent/internal/castai"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"testing"
)

func TestFilter(t *testing.T) {
	tt := map[string]struct {
		event *corev1.Event
		want  bool
	}{
		"returns true if the object is of type *corev1.Event and has the ReportingController field set to AutoscalerController": {
			event: &corev1.Event{
				ReportingController: AutoscalerController,
			},
			want: true,
		},
		"returns false if the object is of type *corev1.Event but does not have the ReportingController field set to AutoscalerController": {
			event: &corev1.Event{
				ReportingController: "something else",
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
