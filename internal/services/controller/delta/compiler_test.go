package delta_test

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/rand"

	"castai-agent/internal/castai"
	"castai-agent/internal/services/controller/delta"
)

func TestCompiler(t *testing.T) {
	type FakeItem struct {
		Nonce string
	}

	testCases := map[string]struct {
		Items0 map[string]string
		Items1 map[string]string

		SkipClear   bool
		FailCompile []string

		Expected0 []string
		Expected1 []string
	}{
		"compiles items": {
			Items0: map[string]string{
				"key-0": "item-A",
				"key-1": "item-B",
			},
			Expected0: []string{"item-A", "item-B"},
		},
		"keeps compiled items if not cleared": {
			Items0: map[string]string{
				"key-0": "item-A",
			},
			Expected0: []string{"item-A"},
			SkipClear: true,
			Items1: map[string]string{
				"key-1": "item-B",
			},
			Expected1: []string{"item-A", "item-B"},
		},
		"deduplicates items based on key": {
			Items0: map[string]string{
				"key-0": "item-A",
				"key-1": "item-B",
				"key-2": "item-C", // Note: value matches but the key doesn't
			},
			Expected0: []string{"item-A", "item-B", "item-C"},
			SkipClear: true,
			Items1: map[string]string{
				"key-1": "item-C",
			},
			Expected1: []string{"item-A", "item-C", "item-C"},
		},
		"skips items that fail to compile": {
			Items0: map[string]string{
				"key-0": "item-A",
			},
			Expected0:   []string{"item-A"},
			SkipClear:   true,
			FailCompile: []string{"item-B"},
			Items1: map[string]string{
				"key-0": "item-B",
			},
			Expected1: []string{"item-A"},
		},
	}

	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			var (
				log            = logrus.New()
				clusterID      = uuid.NewString()
				clusterVersion = uuid.NewString()
				agentVersion   = uuid.NewString()
				fullSnapshot   = rand.Int()%2 == 0
			)

			// Basic translation function to simplify the test cases.
			compiledItems := make(map[string]*castai.DeltaItem)
			compileItemFn := func(input FakeItem) (*castai.DeltaItem, error) {
				if slices.Contains(tt.FailCompile, input.Nonce) {
					return nil, errors.New("fake error")
				}

				// Consistently generated message allows to verify duplicate item values.
				message := json.RawMessage("compiled-" + input.Nonce)
				output := &castai.DeltaItem{
					Data: &message,
				}
				compiledItems[input.Nonce] = output
				return output, nil
			}

			var currentTimeValue time.Time
			timeNowFn := func() time.Time {
				return currentTimeValue
			}

			compiler := delta.NewCompiler[FakeItem](timeNowFn, log, clusterID, clusterVersion, agentVersion, compileItemFn)

			fakeItems0 := lo.MapValues(tt.Items0, func(v string, _ string) FakeItem {
				return FakeItem{v}
			})
			fakeItems1 := lo.MapValues(tt.Items1, func(v string, _ string) FakeItem {
				return FakeItem{v}
			})

			currentTimeValue = time.Now().UTC()

			compiler.Write(fakeItems0)
			delta0 := compiler.AsCastDelta(fullSnapshot)
			require.Equal(t, clusterID, delta0.ClusterID)
			require.Equal(t, clusterVersion, delta0.ClusterVersion)
			require.Equal(t, agentVersion, delta0.AgentVersion)
			require.Equal(t, fullSnapshot, delta0.FullSnapshot)

			expectedItems0 := lo.Map(tt.Expected0, func(v string, _ int) *castai.DeltaItem {
				return compiledItems[v]
			})
			require.ElementsMatch(t, expectedItems0, delta0.Items)
			requireAllCreateAtEqual(t, currentTimeValue, delta0.Items)

			if !tt.SkipClear {
				compiler.Clear()
				compiledItems = nil
			}

			currentTimeValue = time.Now().UTC()

			compiler.Write(fakeItems1)
			delta1 := compiler.AsCastDelta(fullSnapshot)
			require.Equal(t, clusterID, delta1.ClusterID)
			require.Equal(t, clusterVersion, delta1.ClusterVersion)
			require.Equal(t, agentVersion, delta1.AgentVersion)
			require.Equal(t, fullSnapshot, delta1.FullSnapshot)

			expectedItems1 := lo.Map(tt.Expected1, func(v string, _ int) *castai.DeltaItem {
				return compiledItems[v]
			})
			require.ElementsMatch(t, expectedItems1, delta1.Items)
			requireAllCreateAtEqual(t, currentTimeValue, delta1.Items)
		})
	}
}

func requireAllCreateAtEqual(t testing.TB, expected time.Time, items []*castai.DeltaItem) {
	for _, v := range items {
		require.Equal(t, expected, v.CreatedAt)
	}
}
