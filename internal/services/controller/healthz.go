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

	initializeStarted *time.Time
	lastHealthyAction *time.Time
}

func (h *HealthzProvider) Check(_ *http.Request) error {
	if h.lastHealthyAction != nil {
		if time.Since(*h.lastHealthyAction) > h.cfg.Controller.HealthySnapshotIntervalLimit {
			return fmt.Errorf("time since initialization or last snapshot sent is over the considered healthy limit of %s", h.cfg.Controller.HealthySnapshotIntervalLimit)
		}
		return nil
	}

	if h.initializeStarted != nil {
		if time.Since(*h.initializeStarted) > h.initHardTimeout {
			return fmt.Errorf("controller initialization is taking longer than the hard timeout of %s", h.initHardTimeout)
		}
		return nil
	}

	return fmt.Errorf("healthz not initialized")
}

func (h *HealthzProvider) Initializing() {
	if h.initializeStarted == nil {
		h.initializeStarted = nowPtr()
	}
}

func (h *HealthzProvider) Initialized() {
	h.healthyAction()
}

func (h *HealthzProvider) SnapshotSent() {
	h.healthyAction()
}

func (h *HealthzProvider) healthyAction() {
	h.initializeStarted = nil
	h.lastHealthyAction = nowPtr()
}

func nowPtr() *time.Time {
	now := time.Now()
	return &now
}
