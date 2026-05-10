package crd

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// EvictorConfig is a cluster-scoped CR that holds evictor runtime configuration.
type EvictorConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EvictorConfigSpec   `json:"spec,omitempty"`
	Status EvictorConfigStatus `json:"status,omitempty"`
}

// EvictorConfigStatus holds the observed state of EvictorConfig.
type EvictorConfigStatus struct {
	// ObservedGeneration is the .metadata.generation last reconciled by the evictor.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// EvictorConfigSpec holds all evictor configuration.
type EvictorConfigSpec struct {
	// ManagementMode controls who manages this CR.
	// "castai" means CAST AI manages the configuration.
	// "self-managed" means the cluster operator manages it directly.
	ManagementMode string `json:"managementMode"`

	// Enabled is the top-level kill-switch for the evictor.
	// When false, no evictions or node drains are performed regardless of other settings.
	Enabled bool `json:"enabled"`

	// Config holds runtime configuration knobs for the evictor.
	Config EvictorRuntimeConfig `json:"config"`

	// EvictionRules holds per-workload and per-node eviction rules.
	// Each rule targets either pods (via podSelector) or nodes (via nodeSelector) — exactly one must be set.
	// +optional
	EvictionRules []EvictionRule `json:"evictionRules,omitempty"`
}

// EvictorRuntimeConfig holds runtime configuration knobs for the evictor.
type EvictorRuntimeConfig struct {
	// DryRun puts the evictor in simulation mode: all decisions are logged but no
	// evictions, cordons, or soft taints are applied.
	DryRun bool `json:"dryRun"`

	// AggressiveMode targets every eligible pod and node, including single-replica
	// workloads and Jobs that are normally skipped in default mode.
	AggressiveMode bool `json:"aggressiveMode"`

	// ScopedMode restricts the evictor to CAST AI-provisioned nodes only.
	ScopedMode bool `json:"scopedMode"`

	// SoftTainting applies a PreferNoSchedule taint to drain candidates instead of
	// hard-cordoning them, allowing the scheduler to prefer other nodes without fully
	// blocking new pods.
	SoftTainting bool `json:"softTainting"`

	// TargetAllNodes makes the evictor consider all nodes in the cluster as eviction
	// candidates, not just CAST AI-provisioned ones.
	TargetAllNodes bool `json:"targetAllNodes"`

	// EmitNodeRelatedPodEvents emits Kubernetes events on pods when they are evicted
	// as part of a node drain.
	EmitNodeRelatedPodEvents bool `json:"emitNodeRelatedPodEvents"`

	// CleanupKarpenterNodes deletes the Karpenter NodeClaim after draining a
	// Karpenter-managed node, triggering faster node removal.
	CleanupKarpenterNodes bool `json:"cleanupKarpenterNodes"`

	// CycleInterval is the time between consecutive evictor cycles (e.g. "1m", "30s").
	CycleInterval string `json:"cycleInterval"`

	// NodeGracePeriodMinutes is the minimum time a node must be underutilized before
	// the evictor considers it a drain candidate.
	NodeGracePeriodMinutes int32 `json:"nodeGracePeriodMinutes"`

	// DrainTimeout is the maximum time the evictor waits for a node to fully drain
	// before giving up (e.g. "10m").
	DrainTimeout string `json:"drainTimeout"`

	// DrainRollbackTimeout is how long the evictor waits before rolling back a cordon
	// when a drain attempt fails (e.g. "1m").
	DrainRollbackTimeout string `json:"drainRollbackTimeout"`

	// PodEvictionFailureBackOff is the wait time before retrying after a pod eviction
	// request fails (e.g. "5s").
	PodEvictionFailureBackOff string `json:"podEvictionFailureBackOff"`

	// Windows enables eviction of pods and nodes running Windows workloads.
	Windows bool `json:"windows"`

	// IgnorePodDisruptionBudgets disables PDB checks during eviction.
	// Use with caution: this may violate availability guarantees for production workloads.
	IgnorePodDisruptionBudgets bool `json:"ignorePodDisruptionBudgets"`

	// ForceDisableLiveMigration disables LIVE pod migration even when the cluster
	// capability is detected. By default, live migration is used when available.
	ForceDisableLiveMigration bool `json:"forceDisableLiveMigration"`

	// ForceDisableWOOP disables workload-optimized placement recommendations even when
	// the cluster capability is detected.
	ForceDisableWOOP bool `json:"forceDisableWOOP"`

	// ForceDisablePodMutations disables PodMutation CR application during scheduling
	// simulation even when the cluster capability is detected.
	ForceDisablePodMutations bool `json:"forceDisablePodMutations"`

	// ForceDisableKarpenterMode disables Karpenter-aware scheduling simulation even
	// when Karpenter is detected on the cluster.
	ForceDisableKarpenterMode bool `json:"forceDisableKarpenterMode"`

	// MaxTargetNodesPerCycle is the upper bound on nodes drained in a single cycle.
	MaxTargetNodesPerCycle int32 `json:"maxTargetNodesPerCycle"`

	// MinTargetNodesPerCycle is the lower bound on nodes considered per cycle.
	MinTargetNodesPerCycle int32 `json:"minTargetNodesPerCycle"`

	// TargetNodePercentage is the percentage of eligible nodes to consider per cycle,
	// applied after minTargetNodesPerCycle and maxTargetNodesPerCycle bounds.
	TargetNodePercentage int32 `json:"targetNodePercentage"`

	// PricingAwarenessEnabled makes the evictor factor node cost into drain candidate
	// selection, preferring to drain more expensive nodes first.
	PricingAwarenessEnabled bool `json:"pricingAwarenessEnabled"`

	// PricingModel overrides the default cost coefficients used when pricingAwarenessEnabled
	// is true. Zero values fall back to built-in defaults.
	PricingModel EvictorPricingModel `json:"pricingModel"`

	// Arm64Supported indicates arm64 nodes are present so the scheduling simulation
	// includes them as valid bin-packing targets.
	Arm64Supported bool `json:"arm64Supported"`
}

// EvictorPricingModel overrides the cost coefficients used for pricing-aware candidate selection.
type EvictorPricingModel struct {
	// SpotDiscount is the fractional discount applied to spot instance price
	// relative to on-demand (e.g. 0.5 means spot is 50% cheaper).
	SpotDiscount resource.Quantity `json:"spotDiscount"`

	// BaseCPUCost is the cost coefficient per CPU core used in node price estimation.
	BaseCPUCost resource.Quantity `json:"baseCPUCost"`

	// BaseMemCost is the cost coefficient per GiB of memory used in node price estimation.
	BaseMemCost resource.Quantity `json:"baseMemCost"`
}

// EvictionRule is one advanced eviction rule that pairs a selector with behavioral overrides.
type EvictionRule struct {
	// PodSelector targets pods matching the given criteria.
	// Mutually exclusive with nodeSelector.
	// +optional
	PodSelector *EvictionPodSelector `json:"podSelector,omitempty"`

	// NodeSelector targets nodes matching the given label selector.
	// Mutually exclusive with podSelector.
	// +optional
	NodeSelector *EvictionNodeSelector `json:"nodeSelector,omitempty"`

	// Settings defines the eviction behavior applied to matched pods or nodes.
	// nil means no behavioral overrides — the rule only affects selection.
	// +optional
	Settings *EvictionSettings `json:"settings,omitempty"`
}

// EvictionPodSelector matches pods by namespace, owner kind, replica count, and labels.
type EvictionPodSelector struct {
	// Namespace restricts matching to a specific namespace. Omit to match all namespaces.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Kind restricts matching to pods owned by a specific kind (e.g. Deployment, StatefulSet, Job).
	// +optional
	Kind string `json:"kind,omitempty"`

	// ReplicasMin restricts matching to workloads with at least this many replicas.
	// +optional
	ReplicasMin int32 `json:"replicasMin,omitempty"`

	// LabelSelector restricts matching to pods whose labels satisfy this selector.
	// +optional
	LabelSelector *EvictionLabelSelector `json:"labelSelector,omitempty"`
}

// EvictionNodeSelector matches nodes by label selector.
type EvictionNodeSelector struct {
	// LabelSelector restricts matching to nodes whose labels satisfy this selector.
	// +optional
	LabelSelector *EvictionNodeLabelSelector `json:"labelSelector,omitempty"`
}

// EvictionLabelSelector is a Kubernetes-style label selector for pods.
type EvictionLabelSelector struct {
	// +optional
	MatchLabels map[string]string `json:"matchLabels,omitempty"`

	// +optional
	MatchExpressions []EvictionLabelSelectorRequirement `json:"matchExpressions,omitempty"`
}

// EvictionNodeLabelSelector is a Kubernetes-style label selector for nodes.
type EvictionNodeLabelSelector struct {
	// +optional
	MatchLabels map[string]string `json:"matchLabels,omitempty"`

	// +optional
	MatchExpressions []EvictionLabelSelectorRequirement `json:"matchExpressions,omitempty"`
}

// EvictionLabelSelectorRequirement is a label selector requirement with a key, operator, and values.
type EvictionLabelSelectorRequirement struct {
	Key      string `json:"key"`
	Operator string `json:"operator"`
	// +optional
	Values []string `json:"values,omitempty"`
}

// EvictionSettings defines the eviction behavior applied to pods or nodes matched by a rule.
type EvictionSettings struct {
	// RemovalDisabled prevents the evictor from evicting matched pods or draining matched nodes.
	// +optional
	RemovalDisabled bool `json:"removalDisabled,omitempty"`

	// Disposable marks matched pods as evictable regardless of other protection rules
	// (e.g. StatefulSet membership, single-replica guard).
	// +optional
	Disposable bool `json:"disposable,omitempty"`

	// Aggressive applies aggressive-mode eviction to matched pods, overriding the
	// global aggressiveMode setting for this selector only.
	// +optional
	Aggressive bool `json:"aggressive,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EvictorConfigList contains a list of EvictorConfig.
type EvictorConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EvictorConfig `json:"items"`
}
