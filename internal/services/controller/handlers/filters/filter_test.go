package filters

import (
	"testing"

	"github.com/stretchr/testify/require"

	"castai-agent/internal/castai"
)

func TestFilters_Apply(t *testing.T) {
	tt := map[string]struct {
		filters Filters
		want    bool
	}{
		"should return TRUE if 'AND' type filter returns FALSE but 'OR' type filter returns TRUE": {
			filters: Filters{
				{
					func(e castai.EventType, obj interface{}) bool {
						return false
					},

					func(e castai.EventType, obj interface{}) bool {
						return true
					},
				},
				{
					func(e castai.EventType, obj interface{}) bool {
						return true
					},
				},
			},
			want: true,
		},
		"should return FALSE if 'AND' type filter returns FALSE and 'OR' type filter returns FALSE": {
			filters: Filters{
				{
					func(e castai.EventType, obj interface{}) bool {
						return true
					},
					func(e castai.EventType, obj interface{}) bool {
						return false
					},
				},
				{
					func(e castai.EventType, obj interface{}) bool {
						return false
					},
				},
			},
			want: false,
		},
		"should return TRUE if one of 'OR' chain filters return TRUE": {
			filters: Filters{
				{
					func(e castai.EventType, obj interface{}) bool {
						return true
					},
				},
				{
					func(e castai.EventType, obj interface{}) bool {
						return false
					},
				},
			},
			want: true,
		},
		"should return FALSE if one filter of 'AND' returns FALSE": {
			filters: Filters{
				{
					func(e castai.EventType, obj interface{}) bool {
						return true
					},
					func(e castai.EventType, obj interface{}) bool {
						return false
					},
					func(e castai.EventType, obj interface{}) bool {
						return true
					},
				},
			},
			want: false,
		},
	}
	for name, tc := range tt {
		t.Run(name, func(t *testing.T) {
			filters := tc.filters
			apply := filters.Apply(castai.EventAdd, nil)
			require.Equal(t, tc.want, apply)
		})
	}
}
