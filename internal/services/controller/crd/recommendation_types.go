package crd

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TargetRef struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	APIVersion string `json:"apiVersion,omitempty"`
}

type Resources struct {
	ContainerName string          `json:"containerName,omitempty"`
	Requests      v1.ResourceList `json:"requests,omitempty"`
	Limits        v1.ResourceList `json:"limits,omitempty"`
}

// RecommendationSpec defines the desired state of Recommendation
type RecommendationSpec struct {
	TargetRef      TargetRef   `json:"targetRef"`
	Replicas       *int32      `json:"replicas,omitempty"`
	Recommendation []Resources `json:"recommendation"`
}

// RecommendationStatus defines the observed state of Recommendation
type RecommendationStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Recommendation is the Schema for the recommendations API
type Recommendation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RecommendationSpec   `json:"spec,omitempty"`
	Status RecommendationStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RecommendationList contains a list of Recommendation
type RecommendationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Recommendation `json:"items"`
}
