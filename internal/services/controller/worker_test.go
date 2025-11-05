package controller

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestRunControllerWithRestart_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	log := logrus.New()
	log.SetOutput(io.Discard)

	params := &Params{
		Log: log,
	}

	err := RunControllerWithRestart(ctx, params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "context canceled")
}

func TestRepeatUntilContextClosed(t *testing.T) {
	tests := map[string]struct {
		setupContext  func() (context.Context, context.CancelFunc)
		fnBehavior    func(callCount *atomic.Int32) func(context.Context) error
		expectError   bool
		errorContains string
		minCalls      int32
		maxCalls      int32
	}{
		"context cancelled immediately": {
			setupContext: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, cancel
			},
			fnBehavior: func(callCount *atomic.Int32) func(context.Context) error {
				return func(ctx context.Context) error {
					callCount.Add(1)
					return nil
				}
			},
			expectError:   true,
			errorContains: "context canceled",
			minCalls:      0,
			maxCalls:      0,
		},
		"function returns error immediately": {
			setupContext: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 1*time.Second)
			},
			fnBehavior: func(callCount *atomic.Int32) func(context.Context) error {
				return func(ctx context.Context) error {
					callCount.Add(1)
					return fmt.Errorf("test error")
				}
			},
			expectError:   true,
			errorContains: "test error",
			minCalls:      1,
			maxCalls:      1,
		},
		"function succeeds then context cancelled": {
			setupContext: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 100*time.Millisecond)
			},
			fnBehavior: func(callCount *atomic.Int32) func(context.Context) error {
				return func(ctx context.Context) error {
					callCount.Add(1)
					time.Sleep(10 * time.Millisecond)
					return nil
				}
			},
			expectError:   true,
			errorContains: "context deadline exceeded",
			minCalls:      1,
			maxCalls:      20,
		},
		"function errors after multiple calls": {
			setupContext: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 1*time.Second)
			},
			fnBehavior: func(callCount *atomic.Int32) func(context.Context) error {
				return func(ctx context.Context) error {
					count := callCount.Add(1)
					if count >= 3 {
						return fmt.Errorf("error after 3 calls")
					}
					time.Sleep(10 * time.Millisecond)
					return nil
				}
			},
			expectError:   true,
			errorContains: "error after 3 calls",
			minCalls:      3,
			maxCalls:      3,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := tt.setupContext()
			defer cancel()

			var callCount atomic.Int32
			fn := tt.fnBehavior(&callCount)

			err := repeatUntilContextClosed(ctx, fn)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					require.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}

			actualCalls := callCount.Load()
			require.GreaterOrEqual(t, actualCalls, tt.minCalls)
			require.LessOrEqual(t, actualCalls, tt.maxCalls)
		})
	}
}
