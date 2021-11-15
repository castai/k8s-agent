package actions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"

	"castai-agent/cmd/agent-actions/telemetry"
)

type Config struct {
	PollInterval    time.Duration
	PollTimeout     time.Duration
	AckTimeout      time.Duration
	AckRetriesCount int
	AckRetryWait    time.Duration
	ClusterID       string
}

type Service interface {
	Run(ctx context.Context)
}

type ActionHandler interface {
	Handle(ctx context.Context, data []byte) error
}

func NewService(
	log logrus.FieldLogger,
	cfg Config,
	clientset *kubernetes.Clientset,
	telemetryClient telemetry.Client,
) Service {
	return &service{
		log:             log,
		cfg:             cfg,
		telemetryClient: telemetryClient,
		actionHandlers: map[telemetry.AgentActionType]ActionHandler{
			telemetry.AgentActionTypeDeleteNode: newDeleteNodeHandler(log, clientset),
			telemetry.AgentActionTypeDrainNode:  newDrainNodeHandler(log, clientset),
			telemetry.AgentActionTypePatchNode:  newPatchNodeHandler(log, clientset),
		},
	}
}

type service struct {
	log             logrus.FieldLogger
	cfg             Config
	telemetryClient telemetry.Client
	actionHandlers  map[telemetry.AgentActionType]ActionHandler
}

func (s *service) Run(ctx context.Context) {
	for {
		select {
		case <-time.After(s.cfg.PollInterval):
			err := s.doWork(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					s.log.Debug("actions service stopped")
					return
				}

				s.log.Errorf("cycle failed: %v", err)
				continue
			}
		case <-ctx.Done():
			s.log.Debug("actions service stopped")
			return
		}
	}
}

func (s *service) doWork(ctx context.Context) error {
	s.log.Debug("polling actions")
	actions, err := s.pollActions(ctx)
	if err != nil {
		// Skip deadline errors. These are expected for long polling requests.
		if errors.Is(err, context.DeadlineExceeded) {
			s.log.Debugf("no actions returned in given duration=%s, will continue", s.cfg.PollTimeout)
			return nil
		}

		return fmt.Errorf("polling actions: %w", err)
	}

	s.log.Debugf("received actions, len=%d", len(actions))
	if err := s.handleActions(ctx, actions); err != nil {
		return fmt.Errorf("handling actions: %w", err)
	}
	return nil
}

func (s *service) pollActions(ctx context.Context) ([]*telemetry.AgentAction, error) {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.PollTimeout)
	defer cancel()
	actions, err := s.telemetryClient.GetActions(ctx, s.cfg.ClusterID)
	if err != nil {
		return nil, err
	}
	return actions, nil
}

func (s *service) handleActions(ctx context.Context, actions []*telemetry.AgentAction) error {
	for _, action := range actions {
		var err error
		handleErr := s.handleAction(ctx, action)
		ackErr := s.ackAction(ctx, action, handleErr)
		if handleErr != nil {
			err = handleErr
		}
		if ackErr != nil {
			err = fmt.Errorf("%v:%w", err, ackErr)
		}
		if err != nil {
			return fmt.Errorf("action handling failed: %w", err)
		}
	}

	return nil
}

func (s *service) handleAction(ctx context.Context, action *telemetry.AgentAction) (err error) {
	s.log.Infof("handling action, id=%s, type=%s", action.ID, action.Type)
	handler, ok := s.actionHandlers[action.Type]
	if !ok {
		return fmt.Errorf("handler not found for agent action=%s", action.Type)
	}

	if err := handler.Handle(ctx, action.Data); err != nil {
		return err
	}
	return nil
}

func (s *service) ackAction(ctx context.Context, action *telemetry.AgentAction, handleErr error) error {
	s.log.Infof("ack action, id=%s, type=%s", action.ID, action.Type)

	return backoff.RetryNotify(func() error {
		ctx, cancel := context.WithTimeout(ctx, s.cfg.AckTimeout)
		defer cancel()
		return s.telemetryClient.AckActions(ctx, s.cfg.ClusterID, []*telemetry.AgentActionAck{
			{
				ID:    action.ID,
				Error: getHandlerError(handleErr),
			},
		})
	}, backoff.WithContext(
		backoff.WithMaxRetries(
			backoff.NewConstantBackOff(s.cfg.AckRetryWait), uint64(s.cfg.AckRetriesCount),
		),
		ctx,
	), func(err error, duration time.Duration) {
		if err != nil {
			s.log.Debugf("ack failed, will retry: %v", err)
		}
	})
}

func getHandlerError(err error) *string {
	if err != nil {
		str := err.Error()
		return &str
	}
	return nil
}
