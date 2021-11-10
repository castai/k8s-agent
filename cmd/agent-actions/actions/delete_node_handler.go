package actions

import (
	"context"
	"encoding/json"
	"errors"

	"k8s.io/client-go/kubernetes"

	"castai-agent/cmd/agent-actions/telemetry"
)

func newDeleteNodeHandler(clientset kubernetes.Interface) ActionHandler {
	return &deleteNodeHandler{
		clientset: clientset,
	}
}

type deleteNodeHandler struct {
	clientset kubernetes.Interface
}

func (d *deleteNodeHandler) Handle(ctx context.Context, data []byte) error {
	var req telemetry.AgentActionDeleteNode
	if err := json.Unmarshal(data, &req); err != nil {
		return err
	}
	return errors.New("implement me")
}
