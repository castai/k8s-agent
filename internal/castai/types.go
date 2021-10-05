package castai

import (
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

type EKSParams struct {
	ClusterName string `json:"clusterName"`
	Region      string `json:"region"`
	AccountID   string `json:"accountId"`
}

type GKEParams struct {
	Region      string `json:"region"`
	ProjectID   string `json:"projectId"`
	ClusterName string `json:"clusterName"`
	Location    string `json:"location"`
}

type KOPSParams struct {
	CSP         string `json:"cloud"`
	Region      string `json:"region"`
	ClusterName string `json:"clusterName"`
	StateStore  string `json:"stateStore"`
}

type AKSParams struct {
	ClusterName string `json:"clusterName"`
	Region string `json:"region"`
}

type RegisterClusterRequest struct {
	ID   uuid.UUID   `json:"id"`
	Name string      `json:"name"`
	EKS  *EKSParams  `json:"eks"`
	GKE  *GKEParams  `json:"gke"`
	KOPS *KOPSParams `json:"kops"`
	AKS  *AKSParams  `json:"aks"`
}

type Cluster struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
}

type RegisterClusterResponse struct {
	Cluster
}

type IngestAgentLogsRequest struct {
	LogEvent LogEvent `json:"logEvent"`
}

type IngestAgentLogsResponse struct{}

type LogEvent struct {
	Level   string        `json:"level"`
	Time    time.Time     `json:"time"`
	Message string        `json:"message"`
	Fields  logrus.Fields `json:"fields"`
}

type AgentTelemetryRequest struct {
	AgentVersion string `json:"agentVersion"`
	GitCommit    string `json:"gitCommit"`
}

type AgentTelemetryResponse struct {
	IntervalSeconds string `json:"intervalSeconds"`
	Resync          bool   `json:"resync"`
}

type Delta struct {
	ClusterID      string       `json:"clusterId"`
	ClusterVersion string       `json:"clusterVersion"`
	FullSnapshot   bool         `json:"fullSnapshot"`
	Items          []*DeltaItem `json:"items"`
}

type DeltaItem struct {
	Event     EventType `json:"event"`
	Kind      string    `json:"kind"`
	Data      string    `json:"data"`
	CreatedAt time.Time `json:"createdAt"`
}

type EventType string

const (
	EventAdd    EventType = "add"
	EventUpdate EventType = "update"
	EventDelete EventType = "delete"
)
