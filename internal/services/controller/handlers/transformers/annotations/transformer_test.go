package annotations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"castai-agent/internal/castai"
)

func Test_Transform(t *testing.T) {
	var tests = map[string]struct {
		prefixes []string
		maxLen   string
		input    interface{}
		expected interface{}
	}{
		"should do nothing if no annotations": {
			prefixes: []string{},
			maxLen:   "",
			input:    &v1.Pod{ObjectMeta: metav1.ObjectMeta{}},
			expected: &v1.Pod{ObjectMeta: metav1.ObjectMeta{}},
		},
		"should do nothing if pod has valid annotations": {
			prefixes: []string{},
			maxLen:   "",
			input: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"foo.bar/baz": "qux",
				},
			}},
			expected: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"foo.bar/baz": "qux",
				},
			}},
		},
		"should remove annotation when prefix is specified": {
			prefixes: []string{"foo.bar"},
			maxLen:   "",
			input: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
				Annotations: map[string]string{
					"foo.bar/baz": "qux",
					"foo.bar/qux": "baz",
					"cast.ai/foo": "bar",
				},
			}},
			expected: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
				Annotations: map[string]string{
					"cast.ai/foo": "bar",
				},
			}},
		},
		"should not remove annotation when prefix is specified but it is cast.ai annotation": {
			prefixes: []string{"workload.cast.ai"},
			maxLen:   "",
			input: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"workload.cast.ai/foo": "qux",
				},
			}},
			expected: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"workload.cast.ai/foo": "qux",
				},
			}},
		},
		"should not strip annotation when length of value is over the limit but it is cast.ai annotation": {
			prefixes: []string{"workload.cast.ai"},
			maxLen:   "2",
			input: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"workload.cast.ai/foo": "qux",
				},
			}},
			expected: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"workload.cast.ai/foo": "qux",
				},
			}},
		},
		"should strip annotation when length of value is over the limit ": {
			prefixes: []string{},
			maxLen:   "2",
			input: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"foo.bar/baz": "qux",
				},
			}},
			expected: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"foo.bar/baz": "qu"},
			}},
		},
		"should do nothing when length of value is under the limit ": {
			prefixes: []string{},
			maxLen:   "10",
			input: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"foo.bar/baz": "qux",
				},
			}},
			expected: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"foo.bar/baz": "qux"},
			}},
		},
		"should do nothing when length of value is equal to the limit ": {
			prefixes: []string{},
			maxLen:   "3",
			input: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"foo.bar/baz": "qux",
				},
			}},
			expected: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"foo.bar/baz": "qux"},
			}},
		},
		"should default to 2Ki when maxLen is invalid": {
			prefixes: []string{},
			maxLen:   "",
			input: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"foo.bar/baz": strings.Repeat("a", 3000),
				},
			}},
			expected: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"foo.bar/baz": strings.Repeat("a", 2048)},
			}},
		},
		"should not nothing when object is not metav1.Object": {
			prefixes: []string{"foo.bar"},
			maxLen:   "1",
			input:    "foo",
			expected: "foo",
		},
		"should remove annotation when prefix is specified and object is namespace": {
			prefixes: []string{"foo.bar"},
			maxLen:   "",
			input: &v1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"foo.bar/baz": "qux",
				},
			}},
			expected: &v1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{},
			}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			transformer := NewTransformer(tc.prefixes, tc.maxLen)

			for _, event := range []castai.EventType{castai.EventAdd, castai.EventUpdate, castai.EventDelete} {
				e, o := transformer(event, tc.input)
				r.Equal(event, e)
				r.Equal(tc.expected, o)
			}
		})
	}
}
