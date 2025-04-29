package delta

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"castai-agent/internal/castai"
	"castai-agent/internal/services/controller/scheme"
)

func ItemCacheKey(i *Item) (string, error) {
	if len(i.kind) == 0 {
		gvk, err := determineObjectGVK(i.Obj)
		if err != nil {
			return "", fmt.Errorf("failed to determine Object kind: %v", err)
		}
		i.kind = gvk.Kind
	}
	key := fmt.Sprintf("%s::%s/%s", i.kind, i.Obj.GetNamespace(), i.Obj.GetName())
	return key, nil
}

func determineObjectGVK(obj runtime.Object) (schema.GroupVersionKind, error) {
	// If the object contains its TypeMeta, it is expected to be the most accurate value.
	gvk := obj.GetObjectKind().GroupVersionKind()
	if !gvk.Empty() {
		return gvk, nil
	}

	// Some structured objects have their TypeMeta stripped. In that case, it's only possible to look this information based on the runtime type.
	kinds, _, err := scheme.Scheme.ObjectKinds(obj)
	if err != nil {
		return schema.GroupVersionKind{}, fmt.Errorf("failed to find Object %T kind: %v", obj, err)
	}
	if len(kinds) == 0 || len(kinds[0].Kind) == 0 {
		return schema.GroupVersionKind{}, fmt.Errorf("unknown Object kind for Object %T", obj)
	}
	// Typically runtime types resolve to a single GVK combination but if it has multiple, at this point there is no way to
	// determine which was the original one. We are just picking the first one and hoping it works out.
	return kinds[0], nil
}

func ItemCacheCombine(prev, curr *Item) *Item {
	if prev.Event == castai.EventAdd && curr.Event == castai.EventUpdate {
		curr.Event = castai.EventAdd
	} else if prev.Event == castai.EventDelete && (curr.Event == castai.EventAdd || curr.Event == castai.EventUpdate) {
		curr.Event = castai.EventUpdate
	}
	return curr
}

func ItemCacheCompile(item *Item) (*castai.DeltaItem, error) {
	data, err := encodeToJSON(item.Obj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode item object to JSON: %v", err)
	}
	result := &castai.DeltaItem{
		Event: item.Event,
		Kind:  item.kind,
		Data:  data,
	}
	return result, nil
}

func encodeToJSON(obj interface{}) (*json.RawMessage, error) {
	b, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("marshaling %T to json: %v", obj, err)
	}
	o := json.RawMessage(b)
	return &o, nil
}
