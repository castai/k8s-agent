package cleaner

import (
	"regexp"

	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"castai-agent/internal/castai"
)

var (
	// sensitiveValuePattern matches strings which are usually used to name variables holding sensitive values, like
	// passwords. This is a case-insensitive match of the listed words.
	sensitiveValuePattern = regexp.MustCompile(`(?i)passwd|pass|password|pwd|secret|token|key|creds|credential`)
)

func Transformer(e castai.EventType, obj interface{}) (castai.EventType, interface{}) {
	cleanObj(obj)

	return e, obj
}

// cleanObj removes unnecessary or sensitive values from K8s objects.
func cleanObj(obj interface{}) {
	removeManagedFields(obj)
	removeSensitiveEnvVars(obj)
}

func removeManagedFields(obj interface{}) {
	if metaobj, ok := obj.(metav1.Object); ok {
		metaobj.SetManagedFields(nil)
	}
}

func removeSensitiveEnvVars(obj interface{}) {
	var containers []*corev1.Container

	switch o := obj.(type) {
	case *corev1.Pod:
		for i := range o.Spec.Containers {
			containers = append(containers, &o.Spec.Containers[i])
		}
	case *appsv1.Deployment:
		for i := range o.Spec.Template.Spec.Containers {
			containers = append(containers, &o.Spec.Template.Spec.Containers[i])
		}
	case *appsv1.StatefulSet:
		for i := range o.Spec.Template.Spec.Containers {
			containers = append(containers, &o.Spec.Template.Spec.Containers[i])
		}
	case *appsv1.ReplicaSet:
		for i := range o.Spec.Template.Spec.Containers {
			containers = append(containers, &o.Spec.Template.Spec.Containers[i])
		}
	case *appsv1.DaemonSet:
		for i := range o.Spec.Template.Spec.Containers {
			containers = append(containers, &o.Spec.Template.Spec.Containers[i])
		}
	}

	if len(containers) == 0 {
		return
	}

	for _, c := range containers {
		c.Env = lo.Filter(c.Env, func(envVar corev1.EnvVar, _ int) bool {
			return envVar.Value == "" || !sensitiveValuePattern.MatchString(envVar.Name)
		})
	}
}
