package oomevents

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

func TestListOpts(t *testing.T) {
	mockctrl := gomock.NewController(t)
	v := mock_version.NewMockInterface(mockctrl)

	expectedRequirements := fields.Requirements{
		{
			Field:    "involvedObject.kind",
			Operator: selection.Equals,
			Value:    KindPod,
		},
		{
			Field:    "reason",
			Operator: selection.Equals,
			Value:    ReasonOOMEviction,
		},
	}

	opts := metav1.ListOptions{}
	ListOpts(&opts, v)
	_, selector, _ := k8stesting.ExtractFromListOptions(opts)
	if selector == nil {
		selector = fields.Everything()
	}
	require.ElementsMatch(t, expectedRequirements, selector.Requirements())
}
