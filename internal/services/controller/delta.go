package controller

import (
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
	key := keyObject(i.obj)

	if other, ok := d.cache[key]; ok && other.event == eventAdd && i.event == eventUpdate {
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

func encode(obj interface{}) (*json.RawMessage, error) {
	b, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("marshaling %T to json: %v", obj, err)
	}
	// it should allow sending raw json over network without being encoded to base64
	o := json.RawMessage(fmt.Sprintf(`%v`, string(b)))
	return &o, nil
}

type object interface {
	runtime.Object
	metav1.Object
}

type item struct {
	obj   object
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

func keyObject(obj object) string {
	return fmt.Sprintf("%s::%s/%s", reflect.TypeOf(obj).String(), obj.GetNamespace(), obj.GetName())
}
