package crd

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Client generation directives
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PodMutation is the Schema for the podmutations API
type PodMutation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PodMutationSpec   `json:"spec,omitempty"`
	Status PodMutationStatus `json:"status,omitempty"`
}

// PodMutationStatus defines the observed state of PodMutation
type PodMutationStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// PodMutationSpec defines the desired state of PodMutation
type PodMutationSpec struct {
	// Filter defines the criteria for selecting pods to mutate
	Filter Filter `json:"filter"`

	// Patches defines the JSON patches to be applied to the pod
	Patches []JSONPatch `json:"patches"`

	// RestartPolicy defines the policy for restarting the pod
	RestartPolicy RestartPolicy `json:"restartPolicy,omitempty"`

	// Spot defines the spot configuration for the mutation
	Spot SpotConfig `json:"spotConfig,omitempty"`
}

// Filter represents the raw filter data from the pod mutation object
type Filter struct {
	// Workload defines the workload filter
	Workload WorkloadFilter `json:"workload,omitempty"`

	// Pod defines the pod filter
	Pod PodFilter `json:"pod,omitempty"`
}

type WorkloadFilter struct {
	// Kinds defines the types of objects to match
	Kinds []string `json:"kinds,omitempty"`

	// Names defines the names of objects to match
	Names []string `json:"names,omitempty"`

	// Namespaces defines the namespaces to match
	Namespaces []string `json:"namespaces,omitempty"`

	// ExcludeKinds defines patterns which should be excluded from
	// kind matching even if they match patterns in Kinds.
	ExcludeKinds []string `json:"excludeKinds,omitempty"`

	// ExcludeNames defines patterns which should be excluded from
	// name matching even if they match patterns in Names.
	ExcludeNames []string `json:"excludeNames,omitempty"`

	// ExcludeNamespaces defines patterns which should be excluded from
	// namespace matching even if they match patterns in Namespaces.
	ExcludeNamespaces []string `json:"excludeNamespaces,omitempty"`
}

type PodFilter struct {
	// LabelsOperator defines how to combine multiple label filters
	LabelsOperator FilterOperator `json:"labelsOperator,omitempty"`

	// LabelsFilter defines more complex label matching rules
	LabelsFilter []*LabelValue `json:"labelsFilter,omitempty"`

	// ExcludeLabelsOperator defines how to combine multiple labels in ExcludeLabelsFilter
	ExcludeLabelsOperator FilterOperator `json:"excludeLabelsOperator,omitempty"`

	// ExcludeLabelsFilter defines labels that should exclude pods
	// from matching the mutation even if the pod matches labels in LabelsFilter.
	ExcludeLabelsFilter []*LabelValue `json:"excludeLabelsFilter,omitempty"`
}

// FilterOperator defines how to combine multiple filters
type FilterOperator string

const (
	// FilterOperatorUnspecified is used when no operator is specified.
	FilterOperatorUnspecified FilterOperator = ""
	// FilterOperatorAnd requires all conditions to match
	FilterOperatorAnd FilterOperator = "and"
	// FilterOperatorOr requires any condition to match
	FilterOperatorOr FilterOperator = "or"
)

// LabelValue defines a label key-value pair for filtering
type LabelValue struct {
	// Key is the label key
	Key string `json:"key"`
	// Value is the label value
	Value string `json:"value"`
}

// JSONPatchOp defines the type of JSON patch operation
type JSONPatchOp string

const (
	JSONPatchOpAdd     JSONPatchOp = "add"
	JSONPatchOpRemove  JSONPatchOp = "remove"
	JSONPatchOpReplace JSONPatchOp = "replace"
	JSONPatchOpMove    JSONPatchOp = "move"
	JSONPatchOpCopy    JSONPatchOp = "copy"
	JSONPatchOpTest    JSONPatchOp = "test"
)

type JSONPatch struct {
	// Op defines the operation to be performed
	Op JSONPatchOp `json:"op"`

	// Path defines the path to the value to be patched
	Path string `json:"path"`

	// Value defines the value to be patched
	Value apiextensionsv1.JSON `json:"value"`
}

// RestartPolicy defines the policy for restarting the pod when the mutation is applied
type RestartPolicy string

const (
	RestartPolicyDeferred  RestartPolicy = "deferred"
	RestartPolicyImmediate RestartPolicy = "immediate"
)

// SpotMode defines the spot mode for the mutation
type SpotMode string

const (
	SpotModeOptionalSpot  SpotMode = "optional-spot"
	SpotModeOnlySpot      SpotMode = "only-spot"
	SpotModePreferredSpot SpotMode = "preferred-spot"
)

type SpotConfig struct {
	// Mode defines the spot mode for the mutation
	Mode SpotMode `json:"mode,omitempty"`

	// The percentage of pods (0-100) that receive spot scheduling constraints.
	// The specific spot scheduling constraints depend on the selected spot preference.
	// At least the remaining percentage will be scheduled on on-demand instances.
	// This field applies only if SpotPreference is specified.
	DistributionPercentage int `json:"distributionPercentage,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PodMutationList contains a list of PodMutation
type PodMutationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodMutation `json:"items"`
}
