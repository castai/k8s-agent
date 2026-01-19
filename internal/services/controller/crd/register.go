package crd

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// AutoscalingSchemaGroupVersion is the group version used to register these objects
var AutoscalingSchemaGroupVersion = schema.GroupVersion{Group: "autoscaling.cast.ai", Version: "v1"}

// RecommendationGVR is group version resource for recommendation objects
var RecommendationGVR = AutoscalingSchemaGroupVersion.WithResource("recommendations")

// CustomMetricsExporterConfigGVR is group version resource for custom metrics objects
var CustomMetricsExporterConfigGVR = AutoscalingSchemaGroupVersion.WithResource("custommetricsexporterconfigs")

// StorageOptimizationGroupVersion is the group version resource for the storage optimization objects
var StorageOptimizationGroupVersion = schema.GroupVersion{Group: "storageoptimization.cast.ai", Version: "v1alpha1"}

// NodeDiskRecommendationGVR is group version resource for node disk recommendation objects
var NodeDiskRecommendationGVR = StorageOptimizationGroupVersion.WithResource("nodediskrecommendations")

var PodMutationsSchemaGroupVersion = schema.GroupVersion{Group: "pod-mutations.cast.ai", Version: "v1"}

var PodMutationGVR = PodMutationsSchemaGroupVersion.WithResource("podmutations")

// Register types with the SchemeBuilder
var (
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

// Adds the list of known types to the given scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(AutoscalingSchemaGroupVersion,
		&Recommendation{},
		&RecommendationList{},
		&CustomMetricsExporterConfig{},
		&CustomMetricsExporterConfigList{},
	)
	metav1.AddToGroupVersion(scheme, AutoscalingSchemaGroupVersion)

	scheme.AddKnownTypes(StorageOptimizationGroupVersion,
		&NodeDiskRecommendation{},
		&NodeDiskRecommendationList{},
	)
	metav1.AddToGroupVersion(scheme, StorageOptimizationGroupVersion)

	scheme.AddKnownTypes(PodMutationsSchemaGroupVersion,
		&PodMutation{},
		&PodMutationList{},
	)
	metav1.AddToGroupVersion(scheme, PodMutationsSchemaGroupVersion)

	return nil
}
