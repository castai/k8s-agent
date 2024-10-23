package scheme

import (
	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	argorollouts "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"

	"castai-agent/internal/services/controller/crd"
)

var Scheme = runtime.NewScheme()
var builder = runtime.SchemeBuilder{
	corev1.AddToScheme,
	appsv1.AddToScheme,
	storagev1.AddToScheme,
	batchv1.AddToScheme,
	autoscalingv1.AddToScheme,
	v1beta1.AddToScheme,
	policyv1.AddToScheme,
	datadoghqv1alpha1.SchemeBuilder.AddToScheme,
	argorollouts.SchemeBuilder.AddToScheme,
	crd.AddToScheme,
	networkingv1.AddToScheme,
	rbacv1.AddToScheme,
}

func init() {
	utilruntime.Must(builder.AddToScheme(Scheme))
}
