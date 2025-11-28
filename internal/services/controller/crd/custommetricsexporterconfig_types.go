package crd

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeWorkloadConfig defines configuration for node workload metrics collection
type NodeWorkloadConfig struct {
	// DataSource defines the data source configuration for node workload metrics
	DataSource NodeWorkloadDataSource `json:"dataSource"`

	// Metrics defines the list of metrics to collect
	// +kubebuilder:validation:MinItems=1
	Metrics []NodeWorkloadMetric `json:"metrics"`
}

// NodeWorkloadDataSource defines the data source for node workload metrics
type NodeWorkloadDataSource struct {
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
	NodeWorkloads []NodeWorkloadConfig `json:"nodeWorkloads,omitempty"`

	// Prometheus defines the Prometheus metrics configurations
	// +optional
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
	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the CustomMetricsExporterConfig resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
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
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

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
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CustomMetricsExporterConfigList contains a list of CustomMetricsExporterConfig
type CustomMetricsExporterConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []CustomMetricsExporterConfig `json:"items"`
}
