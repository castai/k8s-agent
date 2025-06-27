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
		&PodMutation{},
		&PodMutationList{},
	)
	metav1.AddToGroupVersion(scheme, AutoscalingSchemaGroupVersion)
	return nil
}
