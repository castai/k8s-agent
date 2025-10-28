package controller

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"castai-agent/internal/config"
)

func TestNewHealthzProvider(t *testing.T) {
	t.Run("unhealthy statuses", func(t *testing.T) {
		log := logrus.New()
		cfg := config.Config{Controller: &config.Controller{
			Interval:                       15 * time.Second,
			PrepTimeout:                    time.Millisecond,
			InitialSleepDuration:           time.Millisecond,
			InitializationTimeoutExtension: time.Millisecond,
			HealthySnapshotIntervalLimit:   time.Millisecond,
		}}

		h := NewHealthzProvider(cfg, log)

		t.Run("should return initialize timeout error", func(t *testing.T) {
			h.Initializing()

			time.Sleep(5 * time.Millisecond)

			require.Error(t, h.Check(nil))
		})

		t.Run("should return snapshot timeout error", func(t *testing.T) {
			h.healthyAction()

			time.Sleep(5 * time.Millisecond)

			require.Error(t, h.Check(nil))
		})
	})

	t.Run("healthy statuses", func(t *testing.T) {
		log := logrus.New()
		cfg := config.Config{Controller: &config.Controller{
			Interval:                       15 * time.Second,
			PrepTimeout:                    10 * time.Minute,
			InitialSleepDuration:           30 * time.Second,
			InitializationTimeoutExtension: 5 * time.Minute,
			HealthySnapshotIntervalLimit:   10 * time.Minute,
		}}

		h := NewHealthzProvider(cfg, log)

		t.Run("agent is considered healthy before controller initialization", func(t *testing.T) {
			require.NoError(t, h.Check(nil))
		})

		t.Run("should return no error when still initializing", func(t *testing.T) {
			h.Initializing()

			require.NoError(t, h.Check(nil))
		})

		t.Run("should return no error when timeout after initialization has not yet passed", func(t *testing.T) {
			h.Initialized()

			require.NoError(t, h.Check(nil))
		})

		t.Run("should return no error when time since last snapshot has not been long", func(t *testing.T) {
			h.SnapshotSent()

			require.NoError(t, h.Check(nil))
		})
	})
}
