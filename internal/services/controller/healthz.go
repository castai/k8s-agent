package controller

import (
	"fmt"
	"net/http"
	"time"

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
func (h *HealthzProvider) CheckReadiness(_ *http.Request) error {
	if h.lastHealthyActionAt == nil {
		return fmt.Errorf("controller not initialized or snapshot not sent")
	}
	if time.Since(*h.lastHealthyActionAt) > h.cfg.Controller.HealthySnapshotIntervalLimit {
		return fmt.Errorf("last healthy action is over the healthy limit of %s", h.cfg.Controller.HealthySnapshotIntervalLimit)
	}
	return nil
}

// Liveness: Ok if initialization started, fail if not started or timeout exceeded.
func (h *HealthzProvider) CheckLiveness(_ *http.Request) error {
	if h.initializeStartedAt == nil {
		return fmt.Errorf("controller initialization not started")
	}
	if time.Since(*h.initializeStartedAt) > h.initHardTimeout {
		return fmt.Errorf("controller initialization exceeded hard timeout of %s", h.initHardTimeout)
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

func (h *HealthzProvider) DeltasRead() {
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
