package shutdown_test

import (
	"errors"
	"testing"

	"github.com/sirupsen/logrus"

	"castai-agent/cmd/utils/shutdown"
)

func TestContext(t *testing.T) {
	testCases := map[string]struct {
		triggerErr error
	}{
		"with error": {
			triggerErr: errors.New("test"),
		},
		"without error": {
			triggerErr: nil,
		},
	}

	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			logger := logrus.New()

			ctx, controller := shutdown.Context(t.Context(), logger)

			shutdownTrigger0 := controller.For("trigger 0")
			shutdownTrigger1 := controller.For("trigger 1")
			shutdownTrigger2 := controller.For("trigger 2")

			select {
			case <-ctx.Done():
			default: // OK.
			}

			shutdownTrigger0(tt.triggerErr)

			select {
			case <-ctx.Done(): // OK
			default:
				t.Fatal("context should be done")
			}

			// Repeated triggers should not panic.
			shutdownTrigger1(errors.New("another error"))
			shutdownTrigger2(nil)
		})
	}
}
