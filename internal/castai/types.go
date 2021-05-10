package castai

type EKSParams struct {
	ClusterName string `json:"clusterName"`
	Region      string `json:"region"`
	AccountID   string `json:"accountId"`
}

type RegisterClusterRequest struct {
	Name string    `json:"name"`
	EKS  EKSParams `json:"eks"`
}

type Cluster struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	OrganizationID string    `json:"organizationId"`
	EKS            EKSParams `json:"eks"`
}

type RegisterClusterResponse struct {
	Cluster
}

type AgentCfgResponse struct {
	IntervalSeconds string `json:"intervalSeconds"`
	Resync          bool   `json:"resync"`
}

type Delta struct {
	ClusterID      string       `json:"clusterId"`
	ClusterVersion string       `json:"clusterVersion"`
	Resync         bool         `json:"resync"`
	Items          []*DeltaItem `json:"items"`
}

type DeltaItem struct {
	Event EventType `json:"event"`
	Kind  string    `json:"kind"`
	Data  string    `json:"data"`
}

type EventType string

const (
	EventAdd    EventType = "add"
	EventUpdate EventType = "update"
	EventDelete EventType = "delete"
)
