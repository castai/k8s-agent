package delta_test

import (
	"cmp"
	"errors"
	"testing"

	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"castai-agent/internal/services/controller/delta"
)

func TestBatcher(t *testing.T) {
	type I struct {
		Key      string
		Err      error
		Value    string
		Priority int
	}

	testCases := map[string]struct {
		Items    []I
		Expected []I
	}{
		"empty": {},
		"collects all items": {
			Items: []I{
				{Key: "key-0", Value: "item-A"},
				{Key: "key-1", Value: "item-B"},
				{Key: "key-2", Value: "item-C"},
			},
			Expected: []I{
				{Key: "key-0", Value: "item-A"},
				{Key: "key-1", Value: "item-B"},
				{Key: "key-2", Value: "item-C"},
			},
		},
		"deduplicates based on key": {
			Items: []I{
				{Key: "key-0", Value: "item-A"},
				{Key: "key-1", Value: "item-B"},
				{Key: "key-2", Value: "item-C"},
				{Key: "key-1", Value: "item-D"},
				{Key: "key-2", Value: "item-E"},
			},
			Expected: []I{
				{Key: "key-0", Value: "item-A"},
				{Key: "key-1", Value: "item-D"},
				{Key: "key-2", Value: "item-E"},
			},
		},
		"uses combineFn to determine which duplicated item to keep": {
			Items: []I{
				{Key: "key-0", Value: "item-A", Priority: 1},
				{Key: "key-0", Value: "item-B", Priority: 2},
				{Key: "key-0", Value: "item-C", Priority: 0},
			},
			Expected: []I{
				{Key: "key-0", Value: "item-C", Priority: 0},
			},
		},
		"ignores items for which key cannot be determined": {
			Items: []I{
				{Err: errors.New("fake error")},
				{Key: "key-0", Value: "item-A"},
			},
			Expected: []I{
				{Key: "key-0", Value: "item-A"},
			},
		},
	}

	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			log := logrus.New()
			keyFn := func(item I) (string, error) {
				return item.Key, item.Err
			}
			combineFn := func(prev, curr I) I {
				if r := cmp.Compare(prev.Priority, curr.Priority); r < 0 {
					return prev
				}
				return curr
			}
			batcher := delta.NewBatcher(log, keyFn, combineFn)

			for _, item := range tt.Items {
				batcher.Write(item)
			}
			actual0 := batcher.GetMapAndClear()
			if len(tt.Expected) == 0 {
				require.Empty(t, actual0)
			} else {
				expected0 := lo.SliceToMap(tt.Expected, func(it I) (string, I) {
					return it.Key, it
				})
				require.Equal(t, expected0, actual0)
			}

			actual1 := batcher.GetMapAndClear()
			require.Empty(t, actual1)
		})
	}
}
