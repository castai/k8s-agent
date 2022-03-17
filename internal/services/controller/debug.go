package controller

import (
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/cache"
)

func wrapWithFileSink(path string, log logrus.FieldLogger, handlers cache.ResourceEventHandler) (cache.ResourceEventHandler, io.Closer, error) {
	w, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		return nil, w, err
	}
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			dumpObj(w, log, nil, obj)
			handlers.OnAdd(obj)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			dumpObj(w, log, oldObj, newObj)
			handlers.OnUpdate(oldObj, newObj)
		},
		DeleteFunc: func(obj interface{}) {
			dumpObj(w, log, obj, nil)
			handlers.OnDelete(obj)
		},
	}, w, nil
}

// dumpObj dumps an event with object to a file. Used for debugging purposes.
// if oldObj is nil, event type is CREATE
// if newObj is nil, event type is DELETE
// if neither are nil, event type is UPDATE
func dumpObj(w io.Writer, log logrus.FieldLogger, oldObj, newObj interface{}) {
	e := eventUpdate
	if oldObj == nil {
		e = eventAdd
	}
	if newObj == nil {
		e = eventDelete
	}

	err := json.NewEncoder(w).Encode(struct {
		Event     event
		OldObj    interface{}
		NewObj    interface{}
		UnixMicro int64
	}{
		Event:     e,
		OldObj:    oldObj,
		NewObj:    newObj,
		UnixMicro: time.Now().UnixMicro(),
	})
	if err != nil {
		log.WithField("event_type", e).Warnf("failed to dump event %q: %w", e, err)
	}
}
