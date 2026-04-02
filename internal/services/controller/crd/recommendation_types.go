package crd

import (
	autoscaling "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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

// ContainerAdjustments represents adjustments to be made to a Pod container.
type ContainerAdjustments struct {
	ContainerName    string            `json:"containerName,omitempty"`
	Requests         v1.ResourceList   `json:"requests,omitempty"`
	Limits           v1.ResourceList   `json:"limits,omitempty"`
	Env              EnvVars           `json:"env,omitempty"`
	ResourceStrategy *ResourceStrategy `json:"resourceStrategy,omitempty"`
}

// ResourceStrategyType identifies the resource strategy variant.
type ResourceStrategyType string

const ResourceStrategyTypeNodeAllocatablePercentage ResourceStrategyType = "NodeAllocatablePercentage"

// ResourceStrategy selects a strategy for dynamic resource sizing.
type ResourceStrategy struct {
	Type ResourceStrategyType `json:"type"`

	// +optional
	NodeAllocatablePercentage *NodeAllocatablePercentage `json:"nodeAllocatablePercentage,omitempty"`
}

// NodeAllocatablePercentage represents the node allocatable percentage configuration.
type NodeAllocatablePercentage struct {
	Requests NodeAllocatableRequests `json:"requests"`

	// +optional
	Limits *NodeAllocatableLimits `json:"limits,omitempty"`
}

// NodeAllocatableRequests contains the percentage of node allocatable that the container can consume.
type NodeAllocatableRequests struct {
	CPUPercent    *resource.Quantity `json:"cpuPercent"`
	MemoryPercent *resource.Quantity `json:"memoryPercent"`
}

// NodeAllocatableLimits represents limits on node allocatable resources.
type NodeAllocatableLimits struct {
	CPURatio    *resource.Quantity `json:"cpuRatio,omitempty"`
	MemoryRatio *resource.Quantity `json:"memoryRatio,omitempty"`
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

	// DelaySeconds specifies the number of seconds between the next reconciliation during one-by-one rollout
	// +optional
	DelaySeconds *int32 `json:"delaySeconds,omitempty"`
}

// LimitsPolicyType identifies the limit handling strategy for a single resource.
type LimitsPolicyType string

// ResourceLimitsPolicy defines the limit handling strategy for a single resource.
// +kubebuilder:validation:XValidation:rule="self.type != 'multiplierLimits' || has(self.multiplier)",message="multiplier is required when type is multiplierLimits"
type ResourceLimitsPolicy struct {
	// Type is the limit policy strategy.
	// +kubebuilder:validation:Enum=keepLimits;multiplierLimits;noLimits;maintainRatio
	Type LimitsPolicyType `json:"type"`
	// Multiplier is the factor applied to the recommended request to compute the limit.
	// Required when Type is multiplierLimits. E.g. "1500m" for 1.5x. Must be >= 1.
	// +optional
	// +kubebuilder:validation:XValidation:rule="quantity(self).compareTo(quantity('1')) >= 0",message="must be greater than or equal to 1"
	Multiplier *resource.Quantity `json:"multiplier,omitempty"`
	// OnlyIfOriginalExist skips setting a limit when the container has no current limit.
	// +optional
	OnlyIfOriginalExist *bool `json:"onlyIfOriginalExist,omitempty"`
	// OnlyIfOriginalLower keeps the existing limit when it is already higher than the computed one.
	// +optional
	OnlyIfOriginalLower *bool `json:"onlyIfOriginalLower,omitempty"`
}

// LimitsPolicy defines per-resource limit handling. CPU and memory can have different policies.
type LimitsPolicy struct {
	// CPU defines how the autoscaler handles CPU limits.
	// +optional
	CPU *ResourceLimitsPolicy `json:"cpu,omitempty"`
	// Memory defines how the autoscaler handles memory limits.
	// +optional
	Memory *ResourceLimitsPolicy `json:"memory,omitempty"`
}

// ContainerRuntime identifies the runtime of a container for auto-instrumentation purposes.
// +kubebuilder:validation:Enum=jvm
type ContainerRuntime string

// ContainerRuntimeSpec describes the runtime configuration for a single container.
type ContainerRuntimeSpec struct {
	// Runtime identifies the container runtime (e.g. jvm).
	Runtime ContainerRuntime `json:"runtime"`

	// AutoInstrument controls whether the runtime should be automatically instrumented
	// (e.g. JMX exporter injection for JVM containers).
	// +optional
	AutoInstrument bool `json:"autoInstrument,omitempty"`
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

	// LimitsPolicy defines per-resource limit handling strategies.
	// +optional
	LimitsPolicy *LimitsPolicy `json:"limitsPolicy,omitempty"`

	// RuntimeSpec defines per-container runtime configuration for auto-instrumentation.
	// Key is the container name.
	// +optional
	RuntimeSpec map[string]ContainerRuntimeSpec `json:"runtimeSpec,omitempty"`
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
