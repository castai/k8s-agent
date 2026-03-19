package crd

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ndr,scope=Namespaced

// NodeDiskRecommendation stores disk size recommendations for a single node template
type NodeDiskRecommendation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeDiskRecommendationSpec   `json:"spec,omitempty"`
	Status NodeDiskRecommendationStatus `json:"status,omitempty"`
}

// NodeDiskRecommendationSpec contains disk size recommendations for a specific node template
type NodeDiskRecommendationSpec struct {
	// NodeTemplate is the name/identifier of the node template (e.g., "m5.large")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	NodeTemplate string `json:"nodeTemplate"`

	// Recommendations is an array of disk recommendations for this node template
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Recommendations []DiskRecommendation `json:"recommendations"`
}

// DiskRecommendation contains sizing recommendation for a specific disk
type DiskRecommendation struct {
	// DiskType indicates the type of disk (root or ephemeral)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=ROOT;EPHEMERAL
	DiskType DiskType `json:"diskType"`

	// SizeBytes is the recommended disk size in bytes
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=32212254720
	SizeBytes int64 `json:"sizeBytes"`

	// VolumeType is the recommended EBS volume type (e.g., gp3, io2)
	// +optional
	VolumeType string `json:"volumeType,omitempty"`

	// IOPS is the recommended provisioned IOPS
	// +optional
	// +kubebuilder:validation:Minimum=0
	IOPS int64 `json:"iops,omitempty"`

	// ThroughputMiBps is the recommended throughput in MiB/s
	// +optional
	// +kubebuilder:validation:Minimum=0
	ThroughputMiBps int64 `json:"throughputMiBps,omitempty"`
}

// DiskType represents the type of disk
type DiskType string

const (
	DiskTypeRoot      DiskType = "ROOT"
	DiskTypeEphemeral DiskType = "EPHEMERAL"
)

// NodeDiskRecommendationStatus contains metadata about the recommendation
type NodeDiskRecommendationStatus struct {
	// LastUpdateTime is when the recommendation was last updated
	// +optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NodeDiskRecommendationList contains a list of NodeDiskRecommendation
type NodeDiskRecommendationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeDiskRecommendation `json:"items"`
}
