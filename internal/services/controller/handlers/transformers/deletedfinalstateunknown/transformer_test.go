package deletedfinalstateunknown

import (
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"castai-agent/internal/castai"
)

func Test_Transformer(t *testing.T) {
	tests := []struct {
		name           string
		event          castai.EventType
		object         interface{}
		expectedEvent  castai.EventType
		expectedObject interface{}
	}{
		{
			name:  "should update event type to deleted when object is cache.DeletedFinalStateUnknown",
			event: castai.EventAdd,
			object: cache.DeletedFinalStateUnknown{
				Key: "default/pod",
				Obj: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod",
						Namespace: metav1.NamespaceDefault,
					},
				},
			},
			expectedEvent: castai.EventDelete,
			expectedObject: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod",
					Namespace: metav1.NamespaceDefault,
				},
			},
		},
		{
			name:  "should do nothing when object is not cache.DeletedFinalStateUnknown",
			event: castai.EventAdd,
			object: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod",
					Namespace: metav1.NamespaceDefault,
				},
			},
			expectedEvent: castai.EventAdd,
			expectedObject: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod",
					Namespace: metav1.NamespaceDefault,
				},
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			r := require.New(t)
			event, object := Transformer(test.event, test.object)

			r.Equal(test.expectedEvent, event)
			r.Equal(test.expectedObject, object)
		})
	}
}
