package annotations

import (
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"castai-agent/internal/castai"
	"castai-agent/internal/services/controller/handlers/transformers"
)

const (
	castAnnotationSuffix = "cast.ai"
)

var (
	fallbackMaxLenQuantity = resource.MustParse("2Ki")
)

func NewTransformer(prefixes []string, maxLen string) transformers.Transformer {
	maxLengthQuantity, err := resource.ParseQuantity(maxLen)
	if err != nil {
		maxLengthQuantity = fallbackMaxLenQuantity
	}
	maxLength := int(maxLengthQuantity.Value())

	return func(e castai.EventType, obj interface{}) (castai.EventType, interface{}) {
		cleanObj(obj, prefixes, maxLength)

		return e, obj
	}
}

// cleanObj removes unnecessary annotations from K8s objects.
func cleanObj(obj interface{}, prefixes []string, maxLength int) {
	if metaobj, ok := obj.(metav1.Object); ok {
		annotations := metaobj.GetAnnotations()
		if annotations == nil {
			return
		}
		for key, value := range annotations {
			tokens := strings.Split(key, "/")
			if len(tokens) > 1 && strings.HasSuffix(tokens[0], castAnnotationSuffix) {
				continue
			}

			for _, prefix := range prefixes {
				if strings.HasPrefix(key, prefix) {
					delete(annotations, key)
					continue
				}
			}

			if len(value) > maxLength {
				annotations[key] = value[:maxLength]
				continue
			}
		}

		metaobj.SetAnnotations(annotations)
	}
}
