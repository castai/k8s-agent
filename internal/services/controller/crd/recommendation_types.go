package crd

import (
	autoscaling "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AdjustmentPhase represents the phase of container adjustments (startup or post-startup)
type AdjustmentPhase int

type GroupingOperator string

type GroupingKey string

type TargetRef struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	APIVersion string `json:"apiVersion,omitempty"`
}

// ContainerAdjustments represents the adjustments to be made to a Pod container.
type ContainerAdjustments struct {
	ContainerName string          `json:"containerName,omitempty"`
	Requests      v1.ResourceList `json:"requests,omitempty"`
	Limits        v1.ResourceList `json:"limits,omitempty"`
	Env           EnvVars         `json:"env,omitempty"`
}

type ContainersGrouping []ContainerGrouping

type ContainerGrouping struct {
	// The attribute used to match containers. `name` refers to container name property
	// +kubebuilder:validation:Enum=name;
	Key GroupingKey `json:"key"`

	// Defines how the 'key' is evaluated against the 'values' list
	// +kubebuilder:validation:Enum=contains;
	Operator GroupingOperator `json:"operator"`

	// A list of string values used for matching against the 'key' with the specified operator
	// +kubebuilder:validation:MinItems=1
	Values []string `json:"values"`

	// The target container name into which matching containers should be grouped
	Into string `json:"into"`
}

type RolloutBehavior struct {
	// Defines workload rollout behavior while recommendation is being applied/removed.
	// +kubebuilder:validation:Enum=NoDisruption;Default
	Type string `json:"type"`

	// PreferOneByOne indicates whether to prefer one by one pod rollout behavior when possible.
	// +optional
	PreferOneByOne *bool `json:"preferOneByOne,omitempty"`
}

// RecommendationSpec defines the desired state of Recommendation
type RecommendationSpec struct {
	TargetRef      TargetRef              `json:"targetRef"`
	Replicas       *int32                 `json:"replicas,omitempty"`
	Recommendation []ContainerAdjustments `json:"recommendation"`
	// +optional
	HPA *HPA `json:"hpa,omitempty"`

	// Defines rules for grouping containers based on specific attributes.
	ContainersGrouping ContainersGrouping `json:"containersGrouping,omitempty"`

	// Defines workload rollout behavior while recommendation is being applied/removed.
	RolloutBehavior *RolloutBehavior `json:"rolloutBehavior,omitempty"`

	// Defines workload settings during startup period.
	Startup *Startup `json:"startup,omitempty"`

	// Defines workload settings during post-startup period.
	PostStartup *PostStartup `json:"postStartup,omitempty"`
}

// Startup defines workload settings during startup.
type Startup struct {
	// Defines duration during which startup resources are applied to workload.
	Duration metav1.Duration `json:"duration"`
}

// PostStartup defines workload settings during post-startup.
type PostStartup struct {
	// Defines vertical adjustments hash.
	VerticalHash string `json:"verticalHash"`

	// Defines adjustments applied to workload.
	Recommendation []ContainerAdjustments `json:"recommendation"`
}

type HPA struct {
	Spec *autoscaling.HorizontalPodAutoscalerSpec `json:"spec,omitempty"`
	// +optional
	// +kubebuilder:default=false
	TakeOwnership bool `json:"takeOwnership,omitempty"`
}

// EnvVars represents a list of environment variables to be injected into a container.
// If the same variable name appears multiple times, the last one in the list takes precedence.
type EnvVars []EnvVar

// EnvVar defines a single environment variable to be set in a container during pod creation.
type EnvVar struct {
	// Name of the environment variable.
	Name string `json:"name"`
	// Value of the environment variable.
	Value string `json:"value"`

	// OnConflict defines the strategy to resolve conflicts when a variable with the same name already exists.
	// If not specified, defaults to 'Ignore'.
	// +optional
	// +kubebuilder:default={"type":"Ignore"}
	OnConflict EnvVarOnConflictStrategy `json:"onConflict,omitempty"`
}

// EnvVarOnConflictStrategy provides different strategies to resolve conflicts when a variable with the same name already exists.
// +kubebuilder:validation:XValidation:message="separator must be empty if the type is not Append",rule="!(has(self.separator) && self.type != 'Append')",fieldPath=".separator",reason="FieldValueForbidden"
//
//nolint:lll
type EnvVarOnConflictStrategy struct {
	// Type determines what happens if a variable with the same name already exists in the container.
	// Defaults to 'Ignore' if not specified.
	// +kubebuilder:default=Ignore
	Type EnvVarOnConflict `json:"type,omitempty"`
	// Separator is used to separate the new value from the existing one when OnConflict is set to Append.
	Separator string `json:"separator,omitempty"`
}

// EnvVarOnConflict defines conflict resolution strategies when a variable with the same name already exists.
// +kubebuilder:validation:Type=string
// +kubebuilder:validation:Enum=Ignore;Append
type EnvVarOnConflict string

// RecommendationStatus defines the observed state of Recommendation
type RecommendationStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="VPA Healthy",type="string",JSONPath=".status.conditions[?(@.type=='VPAHealthy')].status"
//+kubebuilder:printcolumn:name="HPA Healthy",type="string",JSONPath=".status.conditions[?(@.type=='HPAHealthy')].status"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Recommendation is the Schema for the recommendations API
type Recommendation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RecommendationSpec   `json:"spec,omitempty"`
	Status RecommendationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RecommendationList contains a list of Recommendation
type RecommendationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Recommendation `json:"items"`
}

type (
	RecommendationConditionType string
	RecommendationStatusReason  string
)
