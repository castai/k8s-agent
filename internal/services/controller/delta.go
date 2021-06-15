package controller

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"castai-agent/internal/castai"
)

// newDelta initializes the delta struct which is used to collect cluster deltas, debounce them and map to CASTAI
// requests.
func newDelta(log logrus.FieldLogger, clusterID, clusterVersion string) *delta {
	return &delta{
		log:            log,
		clusterID:      clusterID,
		clusterVersion: clusterVersion,
		fullSnapshot:   true,
		cache:          map[string]*item{},
	}
}

// delta is used to colelct cluster deltas, debounce them and map to CASTAI requests. It holds a cache of queue items
// which is referenced any time a new item is added to debounce the items.
type delta struct {
	log            logrus.FieldLogger
	clusterID      string
	clusterVersion string
	fullSnapshot   bool
	cache          map[string]*item
}

// add will add an item to the delta cache. It will debounce the objects.
func (d *delta) add(i *item) {
	key := mustKeyObject(i.obj)

	if other, ok := d.cache[key]; ok && other.event == eventAdd && i.event == eventDelete {
		delete(d.cache, key)
	} else if ok && other.event == eventAdd && i.event == eventUpdate {
		i.event = eventAdd
		d.cache[key] = i
	} else if ok && other.event == eventDelete && (i.event == eventAdd || i.event == eventUpdate) {
		i.event = eventUpdate
		d.cache[key] = i
	} else {
		d.cache[key] = i
	}
}

// clear resets the delta cache and sets fullSnapshot to false. Should be called after toCASTAIRequest is successfully
// delivered.
func (d *delta) clear() {
	d.fullSnapshot = false
	d.cache = map[string]*item{}
}

// toCASTAIRequest maps the collected delta cache to the castai.Delta type.
func (d *delta) toCASTAIRequest() *castai.Delta {
	var items []*castai.DeltaItem

	for _, i := range d.cache {
		data, err := encode(i.obj)
		if err != nil {
			d.log.Errorf("failed to encode %T: %v", i.obj, err)
			continue
		}

		kinds, _, err := scheme.ObjectKinds(i.obj)
		if err != nil {
			d.log.Errorf("failed to find object %T kind: %v", i.obj, err)
			continue
		}
		if len(kinds) == 0 || kinds[0].Kind == "" {
			d.log.Errorf("unknown object kind for object %T", i.obj)
			continue
		}

		items = append(items, &castai.DeltaItem{
			Event:     i.event.toCASTAIEvent(),
			Kind:      kinds[0].Kind,
			Data:      data,
			CreatedAt: time.Now().UTC(),
		})
	}

	return &castai.Delta{
		ClusterID:      d.clusterID,
		ClusterVersion: d.clusterVersion,
		FullSnapshot:   d.fullSnapshot,
		Items:          items,
	}
}

func encode(obj interface{}) (string, error) {
	b, err := json.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("marshaling %T to json: %v", obj, err)
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

type item struct {
	obj   runtime.Object
	event event
}

type event string

const (
	eventAdd    event = "add"
	eventDelete event = "delete"
	eventUpdate event = "update"
)

func (e event) toCASTAIEvent() castai.EventType {
	switch e {
	case eventAdd:
		return castai.EventAdd
	case eventDelete:
		return castai.EventDelete
	case eventUpdate:
		return castai.EventUpdate
	}
	return ""
}

// keyObject generates a unique key for an object, for example: `*v1.Pod::namespace/name`.
func keyObject(obj runtime.Object) (string, error) {
	metaObj, ok := obj.(metav1.Object)
	if !ok {
		return "", fmt.Errorf("expected object of type %T to implement metav1.Object", obj)
	}
	return fmt.Sprintf("%s::%s/%s", reflect.TypeOf(obj).String(), metaObj.GetNamespace(), metaObj.GetName()), nil
}

func mustKeyObject(obj runtime.Object) string {
	k, err := keyObject(obj)
	if err != nil {
		panic(fmt.Errorf("getting object key: %w", err))
	}
	return k
}
