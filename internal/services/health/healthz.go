package health

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/samber/lo"
	"github.com/sirupsen/logrus"

	"castai-agent/internal/config"
)

func NewHealthzProvider(cfg config.Config, log logrus.FieldLogger) *HealthzProvider {
	return &HealthzProvider{
		cfg:             cfg,
		log:             log,
		initHardTimeout: cfg.Controller.PrepTimeout + cfg.Controller.InitialSleepDuration + cfg.Controller.InitializationTimeoutExtension,
	}
}

type HealthzProvider struct {
	cfg             config.Config
	log             logrus.FieldLogger
	initHardTimeout time.Duration

	// Time tracking for health checks (monotonic clock support via time.Time)
	initializeStartedAt *time.Time // Current initialization attempt (reset on MarkHealthy)
	lastHealthyActionAt *time.Time // Last time controller was healthy
	firstInitStartedAt  *time.Time // Very first init (never reset, used by startup probe)

	healthMu sync.Mutex
}

// CheckReadiness checks if the controller is ready to serve traffic.
// Only ready if controller has been marked healthy at least once and is still within the healthy interval.
func (h *HealthzProvider) CheckReadiness(r *http.Request) error {
	h.healthMu.Lock()
	defer h.healthMu.Unlock()

	if h.lastHealthyActionAt == nil {
		if h.initializeStartedAt == nil {
			return fmt.Errorf("controller initialization not started")
		}
		return fmt.Errorf("controller not initialized or snapshot not sent")
	}

	// Check if healthy action is still recent
	timeSinceLastHealthy := time.Since(*h.lastHealthyActionAt)
	if timeSinceLastHealthy > h.cfg.Controller.HealthySnapshotIntervalLimit {
		return fmt.Errorf("last healthy action is over the healthy limit of %s", h.cfg.Controller.HealthySnapshotIntervalLimit)
	}

	return nil
}

// CheckLiveness checks if the pod is alive and functioning.
// Passes if:
//   - Controller is healthy (within healthy interval), OR
//   - Controller is still initializing (within init timeout)
func (h *HealthzProvider) CheckLiveness(r *http.Request) error {
	h.healthMu.Lock()
	defer h.healthMu.Unlock()

	// Case 1: Check if we've ever been healthy
	if h.lastHealthyActionAt != nil {
		timeSinceLastHealthy := time.Since(*h.lastHealthyActionAt)
		if timeSinceLastHealthy > h.cfg.Controller.HealthySnapshotIntervalLimit {
			return fmt.Errorf("last healthy action is over the healthy limit of %s", h.cfg.Controller.HealthySnapshotIntervalLimit)
		}
		return nil
	}

	// Case 2: Still initializing - check if within timeout
	if h.initializeStartedAt != nil {
		timeSinceInit := time.Since(*h.initializeStartedAt)
		if timeSinceInit > h.initHardTimeout {
			return fmt.Errorf("controller initialization exceeded hard timeout of %s", h.initHardTimeout)
		}
		return nil
	}

	// Case 3: Neither healthy nor initializing
	return fmt.Errorf("controller not started")
}

// CheckStartup checks if the pod has completed its startup phase.
// This uses the same timeout as liveness but tracks time from the very first
// initialization attempt, allowing for controller restarts during startup without
// triggering pod restarts.
//
// Passes if:
//   - Controller has been marked healthy at least once, OR
//   - First initialization attempt is within timeout
func (h *HealthzProvider) CheckStartup(r *http.Request) error {
	h.healthMu.Lock()
	defer h.healthMu.Unlock()

	// If we've marked healthy at least once, startup is complete
	if h.lastHealthyActionAt != nil {
		return nil
	}

	// If initializing, check if we're within startup timeout from first init
	if h.firstInitStartedAt != nil {
		timeSinceFirstInit := time.Since(*h.firstInitStartedAt)
		if timeSinceFirstInit > h.initHardTimeout {
			return fmt.Errorf("controller startup exceeded timeout of %s", h.initHardTimeout)
		}
		return nil
	}

	// Controller hasn't started initializing yet
	return fmt.Errorf("controller not started")
}

// Initializing marks the controller as starting a new initialization attempt.
// This should be called at the beginning of each controller initialization cycle.
func (h *HealthzProvider) Initializing() {
	h.healthMu.Lock()
	defer h.healthMu.Unlock()

	// Track the very first initialization attempt (never reset, used by startup probe)
	if h.firstInitStartedAt == nil {
		h.firstInitStartedAt = lo.ToPtr(time.Now())
	}

	// Track current initialization attempt (reset on MarkHealthy, used by liveness probe)
	// Always update this to track restarts correctly
	h.initializeStartedAt = lo.ToPtr(time.Now())
	h.lastHealthyActionAt = nil
}

// MarkHealthy marks the controller as healthy.
// This should be called after successfully sending a snapshot or processing deltas.
func (h *HealthzProvider) MarkHealthy() {
	h.healthMu.Lock()
	defer h.healthMu.Unlock()

	// Clear current initialization attempt but keep tracking first init
	h.initializeStartedAt = nil
	h.lastHealthyActionAt = lo.ToPtr(time.Now())
}
