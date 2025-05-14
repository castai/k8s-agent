package castai

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
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
	Region string `json:"region"`
	// NodeResourceGroup resource group where cluster nodes are deployed.
	NodeResourceGroup string `json:"nodeResourceGroup"`
	SubscriptionID    string `json:"subscriptionId"`
}

type OpenshiftParams struct {
	CSP         string `json:"cloud"`
	Region      string `json:"region"`
	ClusterName string `json:"clusterName"`
	InternalID  string `json:"internalId"`
}

type AnywhereParams struct {
	ClusterName           string    `json:"clusterName"`
	KubeSystemNamespaceID uuid.UUID `json:"kubeSystemNamespaceId"`
}

type SelfHostedWithEC2NodesParams struct {
	ClusterName string `json:"clusterName"`
	Region      string `json:"region"`
	AccountID   string `json:"accountId"`
}

type RegisterClusterRequest struct {
	ID                     uuid.UUID                     `json:"id"`
	Name                   string                        `json:"name"`
	EKS                    *EKSParams                    `json:"eks"`
	GKE                    *GKEParams                    `json:"gke"`
	KOPS                   *KOPSParams                   `json:"kops"`
	AKS                    *AKSParams                    `json:"aks"`
	Openshift              *OpenshiftParams              `json:"openshift"`
	Anywhere               *AnywhereParams               `json:"anywhere"`
	SelfHostedWithEC2Nodes *SelfHostedWithEC2NodesParams `json:"self_hosted_with_ec2_nodes"`
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
	Level   string            `json:"level"`
	Time    time.Time         `json:"time"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields"`
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
	ClusterID       string       `json:"clusterId"`
	ClusterVersion  string       `json:"clusterVersion"`
	AgentVersion    string       `json:"agentVersion"`
	FullSnapshot    bool         `json:"fullSnapshot"`
	Items           []*DeltaItem `json:"items"`
	ContinuityToken string       `json:"continuityToken"`
}

type DeltaItem struct {
	Event     EventType        `json:"event"`
	Kind      string           `json:"kind"`
	Data      *json.RawMessage `json:"data"`
	CreatedAt time.Time        `json:"createdAt"`
}

type EventType string

const (
	EventAdd    EventType = "add"
	EventUpdate EventType = "update"
	EventDelete EventType = "delete"
)
