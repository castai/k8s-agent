package telemetry

import (
	"encoding/json"
	"time"
)

type AckAgentActionsRequest struct {
	Ack []*AgentActionAck `json:"ack,omitempty"`
}

type AgentActionAck struct {
	ID    string  `json:"id"`
	Error *string `json:"error,omitempty"`
}

type GetAgentActionsResponse struct {
	Actions []*AgentAction `json:"actions"`
}

type AgentActionType string

const (
	AgentActionTypeDrainNode  AgentActionType = "drain_node"
	AgentActionTypeDeleteNode AgentActionType = "delete_node"
	AgentActionTypePatchNode  AgentActionType = "patch_node"
)

type AgentAction struct {
	ID        string          `json:"id"`
	Type      AgentActionType `json:"type"`
	CreatedAt time.Time       `json:"createdAt"`
	Data      json.RawMessage `json:"data"`
}

type AgentActionDrainNode struct {
	NodeName            string `json:"nodeName"`
	DrainTimeoutSeconds int    `json:"drainTimeoutSeconds"`
	Force               bool   `json:"force"`
}

type AgentActionDeleteNode struct {
	NodeName string `json:"nodeName"`
}

type AgentActionPatchNode struct {
	NodeName string            `json:"nodeName"`
	Labels   map[string]string `json:"labels"`
	Taints   []NodeTaint       `json:"taints"`
}

type NodeTaint struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Effect string `json:"effect"`
}
