package knowngv

import "k8s.io/apimachinery/pkg/runtime/schema"

// We hard-code these values to avoid unnecessary third-party dependencies.
var (
	KarpenterCoreV1Alpha1 = schema.GroupVersion{Group: "karpenter.sh", Version: "v1alpha1"}
	KarpenterCoreV1Alpha5 = schema.GroupVersion{Group: "karpenter.sh", Version: "v1alpha5"}
	KarpenterCoreV1Beta1  = schema.GroupVersion{Group: "karpenter.sh", Version: "v1beta1"}
	KarpenterCoreV1       = schema.GroupVersion{Group: "karpenter.sh", Version: "v1"}
	KarpenterV1Alpha1     = schema.GroupVersion{Group: "karpenter.k8s.aws", Version: "v1alpha1"}
	KarpenterV1Beta1      = schema.GroupVersion{Group: "karpenter.k8s.aws", Version: "v1beta1"}
	KarpenterV1           = schema.GroupVersion{Group: "karpenter.k8s.aws", Version: "v1"}

	RunbooksV1Alpha1 = schema.GroupVersion{Group: "runbooks.cast.ai", Version: "v1alpha1"}

	// LiveV1 is a GroupVersion for Cast AI Container Live Migration resources.
	LiveV1 = schema.GroupVersion{Group: "live.cast.ai", Version: "v1"}

	// KEDAV1Alpha1 is a GroupVersion for KEDA ScaledObject resources.
	KEDAV1Alpha1 = schema.GroupVersion{Group: "keda.sh", Version: "v1alpha1"}

	// VPAAutoscalingV1 is a GroupVersion for VerticalPodAutoscaler resources.
	VPAAutoscalingV1 = schema.GroupVersion{Group: "autoscaling.k8s.io", Version: "v1"}
)
