package actions

import (
	"context"
	"encoding/json"
	"errors"

	"k8s.io/client-go/kubernetes"

	"castai-agent/cmd/agent-actions/telemetry"
)

func newPatchNodeHandler(clientset kubernetes.Interface) ActionHandler {
	return &patchNodeHandler{
		clientset: clientset,
	}
}

type patchNodeHandler struct {
	clientset kubernetes.Interface
}

func (d *patchNodeHandler) Handle(ctx context.Context, data []byte) error {
	var req telemetry.AgentActionPatchNode
	if err := json.Unmarshal(data, &req); err != nil {
		return err
	}
	return errors.New("implement me")
}
