package crd

// WARNING: THIS FILE IS COPIED FROM workload-autoscaler repository! DO NOT EDIT HERE!
// It must be in sync with the original file:
// https://github.com/castai/workload-autoscaler/blob/main/api/v1/custommetricsexporterconfig_types.go

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TransformerNodeMetric string

const (
	TransformerNodeMetricProbes TransformerNodeMetric = "node-probes"
	TransformerNodeMetricPSI    TransformerNodeMetric = "node-psi"
)

// NodeWorkloadConfig defines configuration for node workload metrics collection
type NodeWorkloadConfig struct {
	// DataSource defines the data source configuration for node workload metrics
	DataSource NodeWorkloadDataSource `json:"dataSource"`

	// Metrics defines the list of metrics to collect
	// +kubebuilder:validation:MinItems=1
	Metrics []NodeWorkloadMetric `json:"metrics"`

	// Transformer defines which transformer configuration to use (e.g., "node-probes", "node-psi").
	// This determines the transformer and Avro schema used for metrics processing.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=node-probes;node-psi
	Transformer TransformerNodeMetric `json:"transformer"`

	// KubernetesVersion defines a version constraint for conditional metric collection.
	// Uses semver constraint syntax (e.g., ">= 1.33.0").
	// +optional
	KubernetesVersion string `json:"kubernetesVersion,omitzero"`
}

// NodeWorkloadDataSource defines the data source for node workload metrics
type NodeWorkloadDataSource struct {
	// Name defines a unique name for the node workload data source
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// KubeletPort defines the port to connect to the kubelet
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:default=10250
	KubeletPort int `json:"kubeletPort"`

	// MetricsPath defines the path to the metrics endpoint
	// +kubebuilder:validation:MinLength=1
	MetricsPath string `json:"metricsPath"`

	// Concurrency defines the number of workers to use for fetching metrics from the kubelet
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=10
	Concurrency int `json:"concurrency"`
}

// NodeWorkloadMetric defines a metric to collect from node workload
type NodeWorkloadMetric struct {
	// Name of the metric
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// FilterExpr is an optional expression to filter metrics by their label values.
	// Uses expr language syntax (e.g., 'container != ""', 'namespace != "kube-system"').
	// For details on the supported syntax, see https://expr-lang.org/.
	// +optional
	FilterExpr string `json:"filterExpr,omitzero"`
}

// PrometheusConfig defines configuration for Prometheus metrics collection
type PrometheusConfig struct {
	// DataSource defines the data source configuration for Prometheus metrics
	DataSource PrometheusDataSource `json:"dataSource"`

	// Metrics defines the list of metrics to collect
	// +kubebuilder:validation:MinItems=1
	Metrics []PrometheusMetric `json:"metrics"`
}

// PrometheusDataSource defines the data source for Prometheus metrics
type PrometheusDataSource struct {
	// Name defines a unique name for the Prometheus data source
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// URL defines the Prometheus server URL
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`

	// Timeout defines the timeout for Prometheus queries
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern=`^([0-9]+(\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$`
	// +kubebuilder:default="15s"
	Timeout metav1.Duration `json:"timeout"`
}

// PrometheusMetric defines a metric to collect from Prometheus
type PrometheusMetric struct {
	// Name of the metric
	Name string `json:"name"`

	// Query is the PromQL query to execute
	// +kubebuilder:validation:MinLength=1
	Query string `json:"query"`
}

// CustomMetricsExporterConfigSpec defines the desired state of CustomMetricsExporterConfig
type CustomMetricsExporterConfigSpec struct {
	// +optional
	// MaxItems must be set, otherwise kubernetes rejects it as estimated cost of rule is too high.
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:XValidation:rule="self.all(x, !self.exists(y, x != y && x.dataSource.name == y.dataSource.name))",message="datasource names must be unique"
	NodeWorkloads []NodeWorkloadConfig `json:"nodeWorkloads,omitempty"`

	// Prometheus defines the Prometheus metrics configurations
	// +optional
	// MaxItems must be set, otherwise kubernetes rejects it as estimated cost of rule is too high.
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:XValidation:rule="self.all(x, !self.exists(y, x != y && x.dataSource.name == y.dataSource.name))",message="datasource names must be unique"
	Prometheus []PrometheusConfig `json:"prometheus,omitempty"`

	// Concurrency defines the number of concurrent metric processors
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=10
	Concurrency int `json:"concurrency"`

	// FailureLimit defines the maximum number of consecutive failures before shutdown
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=5
	FailureLimit int `json:"failureLimit"`

	// Interval defines the interval between metric collection cycles
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern=`^([0-9]+(\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$`
	// +kubebuilder:default="15s"
	Interval metav1.Duration `json:"interval"`
}

// CustomMetricsExporterConfigStatus defines the observed state of CustomMetricsExporterConfig.
type CustomMetricsExporterConfigStatus struct {
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastUpdateTime is the timestamp of the last successful configuration update
	// +optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed CustomMetricsExporterConfig
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// DatasourceStatuses represents the status of each data source defined in the CustomMetricsExporterConfig
	// It's inspired by ContainerStatuses in PodStatus
	// It contains both information about the state of the data source and the status of individual metrics within that
	// data source
	// +optional
	// +listType=map
	// +listMapKey=name
	DatasourceStatuses []DatasourceStatus `json:"datasourceStatuses,omitempty"`

	// DatasourceSummary of the overall datasource statuses, e.g., "2/3"
	// +optional
	DatasourceSummary string `json:"datasourceSummary,omitempty"`
}
type DatasourceState string

const (
	DatasourceStateReady  DatasourceState = "DatasourceReady"
	DatasourceStateFailed DatasourceState = "DatasourceFailed"
	DatasourceDisabled    DatasourceState = "DatasourceDisabled"
)

type DatasourceType string

const (
	DatasourceTypeNodeWorkload DatasourceType = "NodeWorkload"
	DatasourceTypePrometheus   DatasourceType = "Prometheus"
)

// DatasourceStatus represents the status of a specific data source within the CustomMetricsExporterConfig
type DatasourceStatus struct {
	// Name of the data source
	// +required
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// +required
	// +kubebuilder:validation:Required
	State DatasourceState `json:"state"`

	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=1024
	Message string `json:"message"`

	// Index of the data source in the CustomMetricsExporterConfig spec arrays
	// +required
	// +kubebuilder:validation:Minimum=0
	Index int `json:"index"`

	// Type of the data source (e.g., "NodeWorkload", "Prometheus")
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=NodeWorkload;Prometheus
	Type DatasourceType `json:"type"`

	// MetricStatuses represents the status of individual metrics within the data source
	// +optional
	// +listType=map
	// +listMapKey=name
	MetricStatuses []MetricStatus `json:"metricStatuses,omitempty"`
}

type MetricState string

const (
	MetricStateFailed MetricState = "MetricFailed"
)

// MetricStatus represents the status of a specific metric within a data source
type MetricStatus struct {
	// Name of the metric
	// +required
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// +required
	// +kubebuilder:validation:Required
	State MetricState `json:"state"`

	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=1024
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Healthy",type="string",JSONPath=".status.conditions[?(@.type=='Healthy')].status"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.datasourceSummary"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// CustomMetricsExporterConfig is the Schema for the custommetrics API
type CustomMetricsExporterConfig struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of CustomMetricsExporterConfig
	// +required
	Spec CustomMetricsExporterConfigSpec `json:"spec"`

	// status defines the observed state of CustomMetricsExporterConfigStatus
	// +optional
	Status CustomMetricsExporterConfigStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// CustomMetricsExporterConfigList contains a list of CustomMetricsExporterConfig
type CustomMetricsExporterConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []CustomMetricsExporterConfig `json:"items"`
}

type (
	CustomMetricsExporterConfigConditionType string
	CustomMetricsExporterConfigStatusReason  string
)

const (
	CustomMetricsExporterConfigConditionTypeHealthy CustomMetricsExporterConfigConditionType = "Healthy"

	ReconciledSuccessfully   CustomMetricsExporterConfigStatusReason = "ReconciledSuccessfully"
	DatasourceCreationFailed CustomMetricsExporterConfigStatusReason = "DatasourceCreationFailed"
)
