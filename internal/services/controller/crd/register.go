package crd

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SchemaGroupVersion is the group version used to register these objects
var SchemaGroupVersion = schema.GroupVersion{Group: "autoscaling.cast.ai", Version: "v1"}

// RecommendationGVR is group version resource for recommendation objects
var RecommendationGVR = SchemaGroupVersion.WithResource("recommendations")

// Register types with the SchemeBuilder
var (
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

// Adds the list of known types to the given scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemaGroupVersion,
		&Recommendation{},
		&RecommendationList{},
	)
	metav1.AddToGroupVersion(scheme, SchemaGroupVersion)
	return nil
}
