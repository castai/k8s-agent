package knowngv

import "k8s.io/apimachinery/pkg/runtime/schema"

// We hard-code these values to avoid unnecessary third-party dependencies.
var (
	KarpenterCoreV1Alpha5 = schema.GroupVersion{Group: "karpenter.sh", Version: "v1alpha5"}
	KarpenterCoreV1Beta1  = schema.GroupVersion{Group: "karpenter.sh", Version: "v1beta1"}
	KarpenterCoreV1       = schema.GroupVersion{Group: "karpenter.sh", Version: "v1"}
	KarpenterV1Alpha1     = schema.GroupVersion{Group: "karpenter.k8s.aws", Version: "v1alpha1"}
	KarpenterV1Beta1      = schema.GroupVersion{Group: "karpenter.k8s.aws", Version: "v1beta1"}
	KarpenterV1           = schema.GroupVersion{Group: "karpenter.k8s.aws", Version: "v1"}

	RunbooksV1Alpha1 = schema.GroupVersion{Group: "runbooks.cast.ai", Version: "v1alpha1"}
)
