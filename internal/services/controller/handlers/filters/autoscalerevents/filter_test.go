package autoscalerevents

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/selection"
	k8stesting "k8s.io/client-go/testing"

	"castai-agent/internal/castai"
	mock_version "castai-agent/internal/services/version/mock"
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
	mockctrl := gomock.NewController(t)
	v := mock_version.NewMockInterface(mockctrl)

	tt := map[string]struct {
		requirements fields.Requirements
		version      string
	}{
		"has requirements in version == 1.19.0": {
			requirements: fields.Requirements{
				{
					Field:    "reportingComponent",
					Operator: selection.Equals,
					Value:    AutoscalerController,
				},
			},
			version: "1.19.0",
		},
		"has requirements in version > 1.19.0": {
			requirements: fields.Requirements{
				{
					Field:    "reportingComponent",
					Operator: selection.Equals,
					Value:    AutoscalerController,
				},
			},
			version: "1.20.0",
		},
		"skips requirements in version < 1.19.0": {
			requirements: fields.Requirements{},
			version:      "1.18.10",
		},
	}
	for name, tc := range tt {
		t.Run(name, func(t *testing.T) {
			v.EXPECT().Full().Return(tc.version)
			opts := metav1.ListOptions{}
			ListOpts(&opts, v)
			_, selector, _ := k8stesting.ExtractFromListOptions(opts)
			if selector == nil {
				selector = fields.Everything()
			}
			require.ElementsMatch(t, tc.requirements, selector.Requirements())
		})
	}

}
