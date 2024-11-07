package delta

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"castai-agent/internal/castai"
	"castai-agent/internal/services/controller/scheme"
)

// New initializes the Delta struct which is used to collect cluster deltas, debounce them and map to CAST AI
// requests.
func New(log logrus.FieldLogger, clusterID, clusterVersion, agentVersion string) *Delta {
	return &Delta{
		log:            log,
		clusterID:      clusterID,
		clusterVersion: clusterVersion,
		agentVersion:   agentVersion,
		FullSnapshot:   true,
		CacheLegacy:    map[string]*Item{},
		CacheModern:    map[string]*Item{},
	}
}

// Delta is used to collect cluster deltas, debounce them and map to CAST AI requests. It holds a Cache of queue items
// which is referenced any time a new Item is added to debounce the items.
type Delta struct {
	log            logrus.FieldLogger
	clusterID      string
	clusterVersion string
	agentVersion   string
	FullSnapshot   bool
	CacheLegacy    map[string]*Item
	CacheModern    map[string]*Item
}

// Add will add an Item to the Delta Cache. It will debounce the objects.
func (d *Delta) Add(i *Item) {
	d.addToLegacy(i)
	d.addToModern(i)
}

func (d *Delta) addToModern(i *Item) {
	cache := d.CacheModern

	if len(i.kind) == 0 {
		gvk, err := determineObjectGVK(i.Obj)
		if err != nil {
			d.log.Errorf("failed to determine Object kind: %v", err)
			return
		}
		i.kind = gvk.Kind
	}

	key := itemCacheKey(i)

	if other, ok := cache[key]; ok && other.event == castai.EventAdd && i.event == castai.EventUpdate {
		i.event = castai.EventAdd
		cache[key] = i
	} else if ok && other.event == castai.EventDelete && (i.event == castai.EventAdd || i.event == castai.EventUpdate) {
		i.event = castai.EventUpdate
		cache[key] = i
	} else {
		cache[key] = i
	}
}

func (d *Delta) addToLegacy(i *Item) {
	cache := d.CacheLegacy

	key := keyObject(i.Obj)

	if other, ok := cache[key]; ok && other.event == castai.EventAdd && i.event == castai.EventUpdate {
		i.event = castai.EventAdd
		cache[key] = i
	} else if ok && other.event == castai.EventDelete && (i.event == castai.EventAdd || i.event == castai.EventUpdate) {
		i.event = castai.EventUpdate
		cache[key] = i
	} else {
		cache[key] = i
	}
}

func keyObject(obj Object) string {
	return fmt.Sprintf("%s::%s/%s", reflect.TypeOf(obj).String(), obj.GetNamespace(), obj.GetName())
}

func itemCacheKey(i *Item) string {
	return fmt.Sprintf("%s::%s/%s", i.kind, i.Obj.GetNamespace(), i.Obj.GetName())
}

// Clear resets the Delta Cache and sets FullSnapshot to false. Should be called after ToCASTAIRequest is successfully
// delivered.
func (d *Delta) Clear() {
	d.FullSnapshot = false
	d.CacheLegacy = map[string]*Item{}
	d.CacheModern = map[string]*Item{}
}

// ToCASTAIRequest maps the collected Delta Cache to the castai.Delta type.
func (d *Delta) ToCASTAIRequest() *castai.Delta {
	resultLegacy := d.toCASTAIRequestLegacy()
	resultModern := d.toCASTAIRequestModern()
	logMismatches(d.log, resultLegacy, resultModern)
	return resultLegacy
}

func (d *Delta) toCASTAIRequestModern() *castai.Delta {
	var items []*castai.DeltaItem

	for _, i := range d.CacheModern {
		data, err := Encode(i.Obj)
		if err != nil {
			d.log.Errorf("failed to encode %T: %v", i.Obj, err)
			continue
		}
		items = append(items, &castai.DeltaItem{
			Event:     i.event,
			Kind:      i.kind,
			Data:      data,
			CreatedAt: time.Now().UTC(),
		})
	}

	return &castai.Delta{
		ClusterID:      d.clusterID,
		ClusterVersion: d.clusterVersion,
		AgentVersion:   d.agentVersion,
		FullSnapshot:   d.FullSnapshot,
		Items:          items,
	}
}

func (d *Delta) toCASTAIRequestLegacy() *castai.Delta {
	var items []*castai.DeltaItem

	for _, i := range d.CacheLegacy {
		data, err := Encode(i.Obj)
		if err != nil {
			d.log.Errorf("failed to encode %T: %v", i.Obj, err)
			continue
		}

		kinds, _, err := scheme.Scheme.ObjectKinds(i.Obj)
		if err != nil {
			d.log.Errorf("failed to find Object %T kind: %v", i.Obj, err)
			continue
		}
		if len(kinds) == 0 || kinds[0].Kind == "" {
			d.log.Errorf("unknown Object kind for Object %T", i.Obj)
			continue
		}

		items = append(items, &castai.DeltaItem{
			Event:     i.event,
			Kind:      kinds[0].Kind,
			Data:      data,
			CreatedAt: time.Now().UTC(),
		})
	}

	return &castai.Delta{
		ClusterID:      d.clusterID,
		ClusterVersion: d.clusterVersion,
		AgentVersion:   d.agentVersion,
		FullSnapshot:   d.FullSnapshot,
		Items:          items,
	}
}

func Encode(obj interface{}) (*json.RawMessage, error) {
	b, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("marshaling %T to json: %v", obj, err)
	}

	o := json.RawMessage(b)
	return &o, nil
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

func logMismatches(log logrus.FieldLogger, legacy *castai.Delta, modern *castai.Delta) {
	if len(legacy.Items) != len(modern.Items) {
		log.Warnf("delta_modern number of items mismatch: %d legacy.Items vs %d modern.Items", len(legacy.Items), len(modern.Items))
	}

	checkLegacy := toChecksumMap(legacy)
	checkModern := toChecksumMap(modern)

	for key, _ := range checkLegacy {
		if checkModern[key] == nil {
			log.Warnf("delta_modern item mismatch: legacy list has item that is missing from modern list: %s", key)
		}
		delete(checkLegacy, key)
		delete(checkModern, key)
	}

	for key, _ := range checkModern {
		if checkLegacy[key] == nil {
			log.Warnf("delta_modern item mismatch: modern list has item that is missing from legacy list: %s", key)
		}
		delete(checkLegacy, key)
		delete(checkModern, key)
	}
}

func toChecksumMap(delta *castai.Delta) map[string]*castai.DeltaItem {
	out := map[string]*castai.DeltaItem{}
	for _, i := range delta.Items {
		var hash string
		if i.Data != nil {
			hash = sha256hash(*i.Data)
		}
		key := fmt.Sprintf("%s-%s-%s", i.Event, i.Kind, hash)
		out[key] = i
	}
	return out
}

func sha256hash(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

type Object interface {
	runtime.Object
	metav1.Object
}

func NewItem(event castai.EventType, obj Object) *Item {
	return &Item{
		Obj:   obj,
		event: event,
	}
}

type Item struct {
	Obj   Object
	event castai.EventType
	kind  string
}
