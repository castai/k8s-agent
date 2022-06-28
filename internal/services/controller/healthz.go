package controller

import (
	"fmt"
	"net/http"
	"time"

	"castai-agent/internal/config"
)

func NewHealthzProvider(cfg config.Config) *HealthzProvider {
	return &HealthzProvider{
		cfg:             cfg,
		initHardTimeout: cfg.Controller.PrepTimeout + cfg.Controller.InitialSleepDuration + cfg.Controller.InitializationTimeoutExtension,
	}
}

type HealthzProvider struct {
	cfg             config.Config
	initHardTimeout time.Duration

	initializeStartedAt *time.Time
	lastHealthyActionAt *time.Time
}

func (h *HealthzProvider) Check(_ *http.Request) error {
	if h.lastHealthyActionAt != nil {
		if time.Since(*h.lastHealthyActionAt) > h.cfg.Controller.HealthySnapshotIntervalLimit {
			return fmt.Errorf("time since initialization or last snapshot sent is over the considered healthy limit of %s", h.cfg.Controller.HealthySnapshotIntervalLimit)
		}
		return nil
	}

	if h.initializeStartedAt != nil {
		if time.Since(*h.initializeStartedAt) > h.initHardTimeout {
			return fmt.Errorf("controller initialization is taking longer than the hard timeout of %s", h.initHardTimeout)
		}
		return nil
	}

	return nil
}

func (h *HealthzProvider) Initializing() {
	if h.initializeStartedAt == nil {
		h.initializeStartedAt = nowPtr()
		h.lastHealthyActionAt = nil
	}
}

func (h *HealthzProvider) Initialized() {
	h.healthyAction()
}

func (h *HealthzProvider) SnapshotSent() {
	h.healthyAction()
}

func (h *HealthzProvider) healthyAction() {
	h.initializeStartedAt = nil
	h.lastHealthyActionAt = nowPtr()
}

func nowPtr() *time.Time {
	now := time.Now()
	return &now
}
