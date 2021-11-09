package actions

import (
	"context"
	"encoding/json"
	"errors"

	"k8s.io/client-go/kubernetes"

	"castai-agent/cmd/agent-actions/telemetry"
)

func newDrainNodeHandler(clientset *kubernetes.Clientset) ActionHandler {
	return &drainNodeHandler{
		clientset: clientset,
	}
}

type drainNodeHandler struct {
	clientset *kubernetes.Clientset
}

func (d *drainNodeHandler) Handle(ctx context.Context, data []byte) error {
	var req telemetry.AgentActionDrainNode
	if err := json.Unmarshal(data, &req); err != nil {
		return err
	}
	return errors.New("implement me")
}
