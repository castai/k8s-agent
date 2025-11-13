package health

import (
	"net/http"
	"testing"
	"time"

	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"castai-agent/internal/config"
)

func TestHealthzProvider_CheckReadiness(t *testing.T) {
	cases := map[string]struct {
		cfg         config.Config
		setup       func(p *HealthzProvider)
		expectError string
	}{
		"fails when initialization not started": {
			cfg: config.Config{
				Controller: &config.Controller{},
			},
			setup:       func(p *HealthzProvider) {},
			expectError: "controller initialization not started",
		},
		"fails when initializing but not initialized": {
			cfg: config.Config{
				Controller: &config.Controller{},
			},
			setup: func(p *HealthzProvider) {
				p.Initializing()
			},
			expectError: "controller not initialized or snapshot not sent",
		},
		"passes immediately after initialized": {
			cfg: config.Config{
				Controller: &config.Controller{
					HealthySnapshotIntervalLimit: 5 * time.Second,
				},
			},
			setup: func(p *HealthzProvider) {
				p.MarkHealthy()
			},
			expectError: "",
		},
		"passes immediately after snapshot sent": {
			cfg: config.Config{
				Controller: &config.Controller{
					HealthySnapshotIntervalLimit: 5 * time.Second,
				},
			},
			setup: func(p *HealthzProvider) {
				p.Initializing()
				p.MarkHealthy()
			},
			expectError: "",
		},
		"passes immediately after deltas read": {
			cfg: config.Config{
				Controller: &config.Controller{
					HealthySnapshotIntervalLimit: 5 * time.Second,
				},
			},
			setup: func(p *HealthzProvider) {
				p.Initializing()
				p.MarkHealthy()
			},
			expectError: "",
		},
		"fails when healthy action exceeds limit": {
			cfg: config.Config{
				Controller: &config.Controller{
					HealthySnapshotIntervalLimit: 1 * time.Millisecond,
				},
			},
			setup: func(p *HealthzProvider) {
				p.MarkHealthy()
				time.Sleep(2 * time.Millisecond)
			},
			expectError: "last healthy action is over the healthy limit",
		},
		"passes at exact limit boundary": {
			cfg: config.Config{
				Controller: &config.Controller{
					HealthySnapshotIntervalLimit: 10 * time.Millisecond,
				},
			},
			setup: func(p *HealthzProvider) {
				p.MarkHealthy()
				time.Sleep(5 * time.Millisecond)
			},
			expectError: "",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			log, _ := test.NewNullLogger()
			provider := NewHealthzProvider(tc.cfg, log)
			tc.setup(provider)
			err := provider.CheckReadiness(&http.Request{})
			if tc.expectError == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectError)
			}
		})
	}
}

func TestHealthzProvider_CheckLiveness(t *testing.T) {
	cases := map[string]struct {
		cfg         config.Config
		setup       func(p *HealthzProvider)
		expectError string
	}{
		"fails when not started": {
			cfg: config.Config{
				Controller: &config.Controller{},
			},
			setup:       func(p *HealthzProvider) {},
			expectError: "controller not started",
		},
		"passes when initializing within timeout": {
			cfg: config.Config{
				Controller: &config.Controller{
					PrepTimeout:                    5 * time.Second,
					InitialSleepDuration:           5 * time.Second,
					InitializationTimeoutExtension: 5 * time.Second,
				},
			},
			setup: func(p *HealthzProvider) {
				p.Initializing()
			},
			expectError: "",
		},
		"fails when initialization exceeds hard timeout": {
			cfg: config.Config{
				Controller: &config.Controller{
					PrepTimeout:                    1 * time.Millisecond,
					InitialSleepDuration:           1 * time.Millisecond,
					InitializationTimeoutExtension: 1 * time.Millisecond,
				},
			},
			setup: func(p *HealthzProvider) {
				p.Initializing()
				time.Sleep(4 * time.Millisecond)
			},
			expectError: "controller initialization exceeded hard timeout",
		},
		"passes when initialized with healthy action within limit": {
			cfg: config.Config{
				Controller: &config.Controller{
					HealthySnapshotIntervalLimit: 5 * time.Second,
				},
			},
			setup: func(p *HealthzProvider) {
				p.MarkHealthy()
			},
			expectError: "",
		},
		"fails when initialized but healthy action exceeds limit": {
			cfg: config.Config{
				Controller: &config.Controller{
					HealthySnapshotIntervalLimit: 1 * time.Millisecond,
				},
			},
			setup: func(p *HealthzProvider) {
				p.MarkHealthy()
				time.Sleep(2 * time.Millisecond)
			},
			expectError: "last healthy action is over the healthy limit",
		},
		"passes after deltas read within limit": {
			cfg: config.Config{
				Controller: &config.Controller{
					HealthySnapshotIntervalLimit: 5 * time.Second,
				},
			},
			setup: func(p *HealthzProvider) {
				p.Initializing()
				p.MarkHealthy()
			},
			expectError: "",
		},
		"passes after snapshot sent within limit": {
			cfg: config.Config{
				Controller: &config.Controller{
					HealthySnapshotIntervalLimit: 5 * time.Second,
				},
			},
			setup: func(p *HealthzProvider) {
				p.Initializing()
				p.MarkHealthy()
			},
			expectError: "",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			log, _ := test.NewNullLogger()
			provider := NewHealthzProvider(tc.cfg, log)
			tc.setup(provider)
			err := provider.CheckLiveness(&http.Request{})
			if tc.expectError == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectError)
			}
		})
	}
}

func TestHealthzProvider_CheckStartup(t *testing.T) {
	cases := map[string]struct {
		cfg         config.Config
		setup       func(p *HealthzProvider)
		expectError string
	}{
		"fails when not started": {
			cfg: config.Config{
				Controller: &config.Controller{},
			},
			setup:       func(p *HealthzProvider) {},
			expectError: "controller not started",
		},
		"passes when initializing within timeout": {
			cfg: config.Config{
				Controller: &config.Controller{
					PrepTimeout:                    5 * time.Second,
					InitialSleepDuration:           5 * time.Second,
					InitializationTimeoutExtension: 5 * time.Second,
				},
			},
			setup: func(p *HealthzProvider) {
				p.Initializing()
			},
			expectError: "",
		},
		"fails when startup exceeds timeout from first init": {
			cfg: config.Config{
				Controller: &config.Controller{
					PrepTimeout:                    1 * time.Millisecond,
					InitialSleepDuration:           1 * time.Millisecond,
					InitializationTimeoutExtension: 1 * time.Millisecond,
				},
			},
			setup: func(p *HealthzProvider) {
				p.Initializing()
				time.Sleep(4 * time.Millisecond)
			},
			expectError: "controller startup exceeded timeout",
		},
		"passes immediately after marked healthy": {
			cfg: config.Config{
				Controller: &config.Controller{
					HealthySnapshotIntervalLimit: 5 * time.Second,
				},
			},
			setup: func(p *HealthzProvider) {
				p.Initializing()
				p.MarkHealthy()
			},
			expectError: "",
		},
		"passes during restart if first init not timed out": {
			cfg: config.Config{
				Controller: &config.Controller{
					PrepTimeout:                    1 * time.Second,
					InitialSleepDuration:           1 * time.Second,
					InitializationTimeoutExtension: 1 * time.Second,
				},
			},
			setup: func(p *HealthzProvider) {
				// First initialization
				p.Initializing()
				p.MarkHealthy()
				// Controller restarts - second initialization
				p.Initializing()
				// Startup should still pass because we're within timeout from first init
			},
			expectError: "",
		},
		"fails if first init timeout exceeded even after restart": {
			cfg: config.Config{
				Controller: &config.Controller{
					PrepTimeout:                    1 * time.Millisecond,
					InitialSleepDuration:           1 * time.Millisecond,
					InitializationTimeoutExtension: 1 * time.Millisecond,
				},
			},
			setup: func(p *HealthzProvider) {
				// First initialization
				p.Initializing()
				time.Sleep(4 * time.Millisecond)
				// Controller restarts - second initialization
				p.Initializing()
				// Startup should fail because first init timeout exceeded
			},
			expectError: "controller startup exceeded timeout",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			log, _ := test.NewNullLogger()
			provider := NewHealthzProvider(tc.cfg, log)
			tc.setup(provider)
			err := provider.CheckStartup(&http.Request{})
			if tc.expectError == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectError)
			}
		})
	}
}
