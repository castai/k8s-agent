package shutdown_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"castai-agent/cmd/utils/shutdown"
)

func TestRunThenTrigger(t *testing.T) {
	testCases := map[string]struct {
		workloadPanic      error
		workloadError      error
		expectCallerPanic  bool
		expectTriggerError error
	}{
		"nil error": {},
		"workload error": {
			workloadError:      errors.New("workload error"),
			expectTriggerError: errors.New("workload error"),
		},
		"workload panic": {
			workloadPanic:      errors.New("workload panic"),
			expectCallerPanic:  true,
			expectTriggerError: errors.New("panic"),
		},
	}

	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			var (
				wg            sync.WaitGroup
				callerPaniced bool
				triggerGot    []error
				trigger       = func(err error) {
					triggerGot = append(triggerGot, err)
				}
			)
			wg.Go(func() {
				defer func() {
					perr := recover()
					callerPaniced = perr != nil
				}()
				shutdown.RunThenTrigger(trigger, true, func() error {
					if tt.workloadPanic != nil {
						panic(tt.workloadPanic)
					}
					return tt.workloadError
				})
			})
			wg.Wait()

			require.Equal(t, tt.expectCallerPanic, callerPaniced)
			require.Len(t, triggerGot, 1)
			require.Equal(t, tt.expectTriggerError, triggerGot[0])
		})
	}
}
