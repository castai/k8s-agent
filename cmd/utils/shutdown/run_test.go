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
		ParamSkipOnSuccess  bool
		WorkloadPanic       error
		WorkloadError       error
		ExpectCallerPanic   bool
		ExpectTriggerErrors []error
	}{
		"nil error": {
			ExpectTriggerErrors: []error{nil},
		},
		"nil error (skipOnSuccess)": {
			ParamSkipOnSuccess:  true,
			ExpectTriggerErrors: nil, // No calls.
		},
		"workload error": {
			WorkloadError:       errors.New("workload error"),
			ExpectTriggerErrors: []error{errors.New("workload error")},
		},
		"workload panic": {
			WorkloadPanic:       errors.New("workload panic"),
			ExpectCallerPanic:   true,
			ExpectTriggerErrors: []error{errors.New("panic")},
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
				shutdown.RunThenTrigger(trigger, tt.ParamSkipOnSuccess, func() error {
					if tt.WorkloadPanic != nil {
						panic(tt.WorkloadPanic)
					}
					return tt.WorkloadError
				})
			})
			wg.Wait()

			require.Equal(t, tt.ExpectCallerPanic, callerPaniced)
			require.Equal(t, len(tt.ExpectTriggerErrors), len(triggerGot))
			for _, err := range tt.ExpectTriggerErrors {
				require.Contains(t, triggerGot, err)
			}
		})
	}
}
