package crd

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Client generation directives
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
//
// Kubebuilder generation directives
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=pomu;pomus
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=`.metadata.annotations.pod-mutations\.cast\.ai/pod-mutation-id`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

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
	// +kubebuilder:validation:Optional
	Filter Filter `json:"filter"`

	// FilterV2 defines the criteria for selecting pods to mutate
	// +kubebuilder:validation:Optional
	FilterV2 FilterV2 `json:"filterV2,omitempty"`

	// Patches defines the JSON patches to be applied to the pod
	// +kubebuilder:validation:Optional
	Patches []JSONPatch `json:"patches"`

	// Patches v2 defines multiple independent sequences of JSON patch operations to be applies to the pod.
	// +kubebuilder:validation:Optional
	PatchesV2 []JSONPatchV2 `json:"patchesV2"`

	// RestartPolicy defines the policy for restarting the pod
	RestartPolicy RestartPolicy `json:"restartPolicy,omitempty"`

	// Spot defines the spot configuration for the mutation
	Spot SpotConfig `json:"spotConfig,omitempty"`

	// DistributionGroups defines percentage-based distribution of configurations across pods
	DistributionGroups []DistributionGroup `json:"distributionGroups,omitempty"`
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
	// LabelsOperator defines how to combine multiple label filters.
	// If not specified, defaults to "and".
	LabelsOperator FilterOperator `json:"labelsOperator,omitempty"`

	// LabelsFilter defines more complex label matching rules
	LabelsFilter []*LabelValue `json:"labelsFilter,omitempty"`

	// ExcludeLabelsOperator defines how to combine multiple labels in ExcludeLabelsFilter.
	// If not specified, defaults to "and".
	ExcludeLabelsOperator FilterOperator `json:"excludeLabelsOperator,omitempty"`

	// ExcludeLabelsFilter defines labels that should exclude pods
	// from matching the mutation even if the pod matches labels in LabelsFilter.
	ExcludeLabelsFilter []*LabelValue `json:"excludeLabelsFilter,omitempty"`
}

// FilterOperator defines how to combine multiple filters
// +kubebuilder:validation:Type=string
// +kubebuilder:validation:Enum=and;or
type FilterOperator string

const (
	// FilterOperatorUnspecified is used when no operator is specified.
	FilterOperatorUnspecified FilterOperator = ""
	// FilterOperatorAnd requires all conditions to match
	FilterOperatorAnd FilterOperator = "and"
	// FilterOperatorOr requires any condition to match
	FilterOperatorOr FilterOperator = "or"
)

func (fo FilterOperator) IsValidOperator() bool {
	return fo == FilterOperatorAnd || fo == FilterOperatorOr || fo == FilterOperatorUnspecified
}

// LabelValue defines a label key-value pair for filtering
type LabelValue struct {
	// Key is the label key
	Key string `json:"key"`
	// Value is the label value
	Value string `json:"value"`
}

// Filter represents the raw filter data from the pod mutation object
type FilterV2 struct {
	// Workload defines the workload filter
	Workload WorkloadFilterV2 `json:"workload,omitempty"`

	// Pod defines the pod filter
	Pod PodFilterV2 `json:"pod,omitempty"`
}

// MatcherType defines the type of matcher
// +kubebuilder:validation:Type=string
// +kubebuilder:validation:Enum=exact;regex
type MatcherType string

const (
	MatcherTypeExact MatcherType = "exact"
	MatcherTypeRegex MatcherType = "regex"
)

type Matcher struct {
	// Type is the type of matcher
	Type MatcherType `json:"type"`

	// Value is the value to match
	Value string `json:"value"`
}

type WorkloadFilterV2 struct {
	// Kinds defines the types of objects to match
	Kinds []Matcher `json:"kinds,omitempty"`

	// Names defines the names of objects to match
	Names []Matcher `json:"names,omitempty"`

	// Namespaces defines the namespaces to match
	Namespaces []Matcher `json:"namespaces,omitempty"`

	// ExcludeKinds defines patterns which should be excluded from
	// kind matching even if they match patterns in Kinds.
	ExcludeKinds []Matcher `json:"excludeKinds,omitempty"`

	// ExcludeNames defines patterns which should be excluded from
	// name matching even if they match patterns in Names.
	ExcludeNames []Matcher `json:"excludeNames,omitempty"`

	// ExcludeNamespaces defines patterns which should be excluded from
	// namespace matching even if they match patterns in Namespaces.
	ExcludeNamespaces []Matcher `json:"excludeNamespaces,omitempty"`
}

type LabelMatcher struct {
	// KeyMatcher is the key to match
	KeyMatcher Matcher `json:"keyMatcher"`
	// ValueMatcher is the value to match
	ValueMatcher Matcher `json:"valueMatcher"`
}

type LabelsFilter struct {
	// Operator defines how to combine multiple label filters
	Operator FilterOperator `json:"operator,omitempty"`

	// Matchers defines more complex label matching rules
	Matchers []LabelMatcher `json:"matchers,omitempty"`
}

type PodFilterV2 struct {
	// Labels defines labels that should match pods
	// for the mutation to be applied.
	Labels LabelsFilter `json:"labels,omitempty"`

	// ExcludeLabels defines labels that should exclude pods
	// from matching the mutation even if the pod matches labels in Labels.
	ExcludeLabels LabelsFilter `json:"excludeLabels,omitempty"`
}

// JSONPatchOp defines the type of JSON patch operation
// +kubebuilder:validation:Type=string
// +kubebuilder:validation:Enum=add;remove;replace;move;copy;test
type JSONPatchOp string

const (
	JSONPatchOpAdd     JSONPatchOp = "add"
	JSONPatchOpRemove  JSONPatchOp = "remove"
	JSONPatchOpReplace JSONPatchOp = "replace"
	JSONPatchOpMove    JSONPatchOp = "move"
	JSONPatchOpCopy    JSONPatchOp = "copy"
	JSONPatchOpTest    JSONPatchOp = "test"
)

func (o JSONPatchOp) String() string { return string(o) }

type JSONPatch struct {
	// Op defines the operation to be performed
	// +kubebuilder:validation:Required
	Op JSONPatchOp `json:"op"`

	// Path defines the path to the value to be patched
	// +kubebuilder:validation:Required
	Path string `json:"path"`

	// Value defines the value to be patched
	// +kubebuilder:validation:Required
	Value apiextensionsv1.JSON `json:"value"`
}

type JSONPatchV2 struct {
	// Operations defines a sequence of JSON patch operations to be applied in order
	// +kubebuilder:validation:Required
	Operations []JSONPatchOperation `json:"operations"`
}

type JSONPatchOperation struct {
	// Op defines the operation to be performed
	// +kubebuilder:validation:Required
	Op JSONPatchOp `json:"op"`

	// Path defines the path to the value to be patched
	// +kubebuilder:validation:Required
	Path string `json:"path"`

	// Value defines the value to be patched
	// +kubebuilder:validation:Optional
	Value apiextensionsv1.JSON `json:"value"`

	// From defines the source path for move and copy operations
	From string `json:"from"`
}

// RestartPolicy defines the policy for restarting the pod when the mutation is applied
// +kubebuilder:validation:Type=string
// +kubebuilder:validation:Enum=deferred;immediate
type RestartPolicy string

const (
	RestartPolicyDeferred  RestartPolicy = "deferred"
	RestartPolicyImmediate RestartPolicy = "immediate"
)

// SpotMode defines the spot mode for the mutation
// +kubebuilder:validation:Type=string
// +kubebuilder:validation:Enum=optional-spot;only-spot;preferred-spot
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
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:validation:ExclusiveMinimum=false
	// +kubebuilder:validation:ExclusiveMaximum=false
	DistributionPercentage int `json:"distributionPercentage,omitempty"`
}

// DistributionGroup defines a percentage-based distribution group for configurations
type DistributionGroup struct {
	// Name uniquely identifies the distribution group
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Percentage defines the percentage of pods (0-100) that should receive this configuration
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:validation:ExclusiveMinimum=false
	// +kubebuilder:validation:ExclusiveMaximum=false
	Percentage int `json:"percentage"`

	// Configuration defines the specific configuration to apply to pods in this distribution group
	// +kubebuilder:validation:Required
	Configuration DistributionGroupConfiguration `json:"configuration"`
}

// DistributionGroupConfiguration defines the configuration that will be applied to pods within a distribution group
type DistributionGroupConfiguration struct {
	// SpotMode defines the spot mode for pods in this distribution group
	SpotMode SpotMode `json:"spotMode,omitempty"`

	// Patches defines JSON patches to apply to pods in this distribution group
	Patches []JSONPatch `json:"patches,omitempty"`

	// PatchesV2 defines multiple independent sequences of JSON patch operations to be applies to the pod
	PatchesV2 []JSONPatchV2 `json:"patchesV2,omitempty"`
}

// +kubebuilder:object:root=true

// PodMutationList contains a list of PodMutation
type PodMutationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodMutation `json:"items"`
}
