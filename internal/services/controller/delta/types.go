package delta

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"castai-agent/internal/castai"
)

type Item struct {
	Obj        Object
	Event      castai.EventType
	kind       string
	receivedAt time.Time
}

type Object interface {
	runtime.Object
	metav1.Object
}

func NewItem(event castai.EventType, obj Object) *Item {
	return &Item{
		Obj:        obj,
		Event:      event,
		receivedAt: time.Now().UTC(),
	}
}
