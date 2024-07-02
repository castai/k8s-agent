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

// New initializes the Delta struct which is used to collect cluster deltas, debounce them and map to CAST AI
// requests.
func New(log logrus.FieldLogger, clusterID, clusterVersion, agentVersion string) *Delta {
	return &Delta{
		log:            log,
		clusterID:      clusterID,
		clusterVersion: clusterVersion,
		agentVersion:   agentVersion,
		FullSnapshot:   true,
		Cache:          map[string]*Item{},
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
	Cache          map[string]*Item
}

// Add will add an Item to the Delta Cache. It will debounce the objects.
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

func keyObject(obj Object) string {
	return fmt.Sprintf("%s::%s/%s", reflect.TypeOf(obj).String(), obj.GetNamespace(), obj.GetName())
}

// Clear resets the Delta Cache and sets FullSnapshot to false. Should be called after ToCASTAIRequest is successfully
// delivered.
func (d *Delta) Clear() {
	d.FullSnapshot = false
	d.Cache = map[string]*Item{}
}

// ToCASTAIRequest maps the collected Delta Cache to the castai.Delta type.
func (d *Delta) ToCASTAIRequest() *castai.Delta {
	var items []*castai.DeltaItem

	for _, i := range d.Cache {
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
