package controller

import (
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

var scheme = runtime.NewScheme()
var builder = runtime.SchemeBuilder{
	corev1.AddToScheme,
	appsv1.AddToScheme,
	storagev1.AddToScheme,
	batchv1.AddToScheme,
}

func init() {
	utilruntime.Must(builder.AddToScheme(scheme))
}
