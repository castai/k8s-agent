package delta

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"castai-agent/internal/castai"
	"castai-agent/internal/services/controller/scheme"
)

// New initializes the Delta struct which is used to collect cluster deltas, debounce them and map to CASTAI
// requests.
func New(log logrus.FieldLogger, clusterID, clusterVersion string) *Delta {
	return &Delta{
		log:            log,
		clusterID:      clusterID,
		clusterVersion: clusterVersion,
		fullSnapshot:   true,
		Cache:          map[string]*Item{},
	}
}

// Delta is used to colelct cluster deltas, debounce them and map to CASTAI requests. It holds a Cache of queue items
// which is referenced any time a new Item is added to debounce the items.
type Delta struct {
	log            logrus.FieldLogger
	clusterID      string
	clusterVersion string
	fullSnapshot   bool
	Cache          map[string]*Item
}

func (d *Delta) IsFullSnapshot() bool {
	return d.fullSnapshot
}

// Add will Add an Item to the Delta Cache. It will debounce the objects.
func (d *Delta) Add(i *Item) {
	key := keyObject(i.Obj)

	if other, ok := d.Cache[key]; ok && other.event == castai.EventAdd && i.event == castai.EventUpdate {
		i.event = castai.EventAdd
		d.Cache[key] = i
	} else if ok && other.event == castai.EventDelete && (i.event == castai.EventAdd || i.event == castai.EventUpdate) {
		i.event = castai.EventUpdate
		d.Cache[key] = i
	} else {
		d.Cache[key] = i
	}
}

// clear resets the Delta Cache and sets fullSnapshot to false. Should be called after toCASTAIRequest is successfully
// delivered.
func (d *Delta) Clear() {
	d.fullSnapshot = false
	d.Cache = map[string]*Item{}
}

// toCASTAIRequest maps the collected Delta Cache to the castai.Delta type.
func (d *Delta) ToCASTAIRequest() *castai.Delta {
	var items []*castai.DeltaItem

	for _, i := range d.Cache {
		data, err := encode(i.Obj)
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
	o := json.RawMessage(b)
	return &o, nil
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
}

func keyObject(obj Object) string {
	return fmt.Sprintf("%s::%s/%s", reflect.TypeOf(obj).String(), obj.GetNamespace(), obj.GetName())
}
