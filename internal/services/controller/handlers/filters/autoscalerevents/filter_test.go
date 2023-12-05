package autoscalerevents

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/selection"
	k8stesting "k8s.io/client-go/testing"

	"castai-agent/internal/castai"
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

func TestListOpts(t *testing.T) {
	expectedRequirements := fields.Requirements{
		{
			Field:    "reportingComponent",
			Operator: selection.Equals,
			Value:    AutoscalerController,
		},
	}

	opts := metav1.ListOptions{}
	ListOpts(&opts)
	_, selector, _ := k8stesting.ExtractFromListOptions(opts)
	if selector == nil {
		selector = fields.Everything()
	}
	require.ElementsMatch(t, expectedRequirements, selector.Requirements())
}
