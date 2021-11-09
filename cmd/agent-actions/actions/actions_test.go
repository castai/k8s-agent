package actions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"castai-agent/cmd/agent-actions/telemetry"
)

func TestActions(t *testing.T)  {
	r := require.New(t)

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	cfg := Config{
		PollInterval: 1 * time.Millisecond,
		ClusterID:    uuid.New().String(),
	}


	newTestService := func(handler ActionHandler, telemetryClient telemetry.Client) Service {
		svc := NewService(log, cfg, nil, telemetryClient)
		svc.(*service).actionHandlers = map[telemetry.AgentActionType]ActionHandler{
			telemetry.AgentActionTypePatchNode: handler,
		}
		return svc
	}

	t.Run("poll, handle and ack", func(t *testing.T) {
		apiActions := []*telemetry.AgentAction{
			{
				ID:        "a1",
				Type:      telemetry.AgentActionTypePatchNode,
				CreatedAt: time.Now(),
				Data:      []byte("json"),
			},
			{
				ID:        "b1",
				Type:      telemetry.AgentActionTypePatchNode,
				CreatedAt: time.Now(),
				Data:      []byte("json"),
			},
		}
		telemetryClient := newMockTelemetryClient(apiActions)
		handler := &mockAgentActionHandler{}
		svc := newTestService(handler, telemetryClient)
		ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Millisecond)
		defer func() {
			cancel()
			r.Len(telemetryClient.acks, 2)
			r.Equal("a1", telemetryClient.acks[0].ID)
			r.Equal("b1", telemetryClient.acks[1].ID)
		}()
		svc.Run(ctx)
	})

	t.Run("ack with error when action handler failed", func(t *testing.T) {
		apiActions := []*telemetry.AgentAction{
			{
				ID:        "a1",
				Type:      telemetry.AgentActionTypePatchNode,
				CreatedAt: time.Now(),
				Data:      []byte("json"),
			},
		}
		telemetryClient := newMockTelemetryClient(apiActions)
		handler := &mockAgentActionHandler{err: errors.New("ups")}
		svc := newTestService(handler, telemetryClient)
		ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Millisecond)
		defer func() {
			cancel()
			r.Empty(telemetryClient.actions)
			r.Len(telemetryClient.acks, 1)
			r.Equal("a1", telemetryClient.acks[0].ID)
			r.Equal("ups", *telemetryClient.acks[0].Error)
		}()
		svc.Run(ctx)
	})
}

type mockAgentActionHandler struct {
	err error
}

func (m *mockAgentActionHandler) Handle(ctx context.Context, data []byte) error {
	return m.err
}

func newMockTelemetryClient(actions []*telemetry.AgentAction) *mockTelemetryClient {
	return &mockTelemetryClient{actions: actions}
}

type mockTelemetryClient struct {
	actions []*telemetry.AgentAction
	acks []*telemetry.AgentActionAck
}

func (m *mockTelemetryClient) GetActions(ctx context.Context, clusterID string) ([]*telemetry.AgentAction, error) {
	return m.actions, nil
}

func (m *mockTelemetryClient) AckActions(ctx context.Context, clusterID string, ack []*telemetry.AgentActionAck) error {
	m.removeAckedActions(ack)

	m.acks = append(m.acks, ack...)
	return nil
}

func (m *mockTelemetryClient) removeAckedActions(ack []*telemetry.AgentActionAck)  {
	var remaining []*telemetry.AgentAction
	isAcked := func(id string) bool {
		for _, actionAck := range ack {
			if actionAck.ID == id {
				return true
			}
		}
		return false
	}
	for _, action := range m.actions {
		if !isAcked(action.ID) {
			remaining = append(remaining, action)
		}
	}
	m.actions = remaining
}
