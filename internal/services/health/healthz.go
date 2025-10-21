package health

import (
	"fmt"
	"net/http"
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

	initializeStartedAt *time.Time
	lastHealthyActionAt *time.Time
}

// Readiness: Only ready if lastHealthyActionAt is set and within healthy interval.
func (h *HealthzProvider) CheckReadiness(r *http.Request) error {
	if h.lastHealthyActionAt == nil {
		// Add more detailed logging to understand the state
		if h.initializeStartedAt == nil {
			h.log.Debug("readiness check failed: controller initialization not started")
			return fmt.Errorf("controller initialization not started")
		}
		h.log.Debug("readiness check failed: controller not initialized or snapshot not sent")
		return fmt.Errorf("controller not initialized or snapshot not sent")
	}

	timeSinceLastHealthy := time.Since(*h.lastHealthyActionAt)
	if timeSinceLastHealthy > h.cfg.Controller.HealthySnapshotIntervalLimit {
		h.log.WithField("time_since_last_healthy", timeSinceLastHealthy).
			WithField("healthy_limit", h.cfg.Controller.HealthySnapshotIntervalLimit).
			Debug("readiness check failed: last healthy action exceeded limit")
		return fmt.Errorf("last healthy action is over the healthy limit of %s", h.cfg.Controller.HealthySnapshotIntervalLimit)
	}

	h.log.Debug("readiness check passed")
	return nil
}

// Liveness: Ok if initializing (within timeout) or if initialized with recent healthy action.
func (h *HealthzProvider) CheckLiveness(r *http.Request) error {
	// Case 1: Initialized - check for recent healthy action (check this first)
	if h.lastHealthyActionAt != nil {
		timeSinceLastHealthy := time.Since(*h.lastHealthyActionAt)
		if timeSinceLastHealthy > h.cfg.Controller.HealthySnapshotIntervalLimit {
			h.log.WithField("time_since_last_healthy", timeSinceLastHealthy).
				WithField("healthy_limit", h.cfg.Controller.HealthySnapshotIntervalLimit).
				Debug("liveness check failed: last healthy action exceeded limit")
			return fmt.Errorf("last healthy action is over the healthy limit of %s", h.cfg.Controller.HealthySnapshotIntervalLimit)
		}
		h.log.Debug("liveness check passed: healthy")
		return nil
	}

	// Case 2: Still initializing
	if h.initializeStartedAt != nil {
		timeSinceInit := time.Since(*h.initializeStartedAt)
		if timeSinceInit > h.initHardTimeout {
			h.log.WithField("time_since_init", timeSinceInit).
				WithField("hard_timeout", h.initHardTimeout).
				Debug("liveness check failed: initialization timeout exceeded")
			return fmt.Errorf("controller initialization exceeded hard timeout of %s", h.initHardTimeout)
		}
		h.log.Debug("liveness check passed: initializing")
		return nil
	}

	// Case 3: Neither initializing nor initialized
	h.log.Debug("liveness check failed: controller not started")
	return fmt.Errorf("controller not started")
}

func (h *HealthzProvider) Initializing() {
	if h.initializeStartedAt == nil {
		h.initializeStartedAt = lo.ToPtr(time.Now())
		h.lastHealthyActionAt = nil
	}
}

func (h *HealthzProvider) Initialized() {
	h.healthyAction()
}

func (h *HealthzProvider) DeltasRead() {
	h.healthyAction()
}

func (h *HealthzProvider) SnapshotSent() {
	h.healthyAction()
}

func (h *HealthzProvider) healthyAction() {
	h.initializeStartedAt = nil
	h.lastHealthyActionAt = lo.ToPtr(time.Now())
}
