package cast

import "castai-agent/internal/services/collector"

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

type Request struct {
	Payload []byte `json:"payload"`
}

type Snapshot struct {
	ClusterID       string `json:"clusterId"`
	AccountID       string `json:"accountId"`
	OrganizationID  string `json:"organizationId"`
	ClusterProvider string `json:"clusterProvider"`
	ClusterName     string `json:"clusterName"`
	ClusterVersion  string `json:"clusterVersion"`
	ClusterRegion   string `json:"clusterRegion"`
	*collector.ClusterData
}
