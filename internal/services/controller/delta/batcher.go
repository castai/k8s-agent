package delta

import (
	"sync"

	"github.com/sirupsen/logrus"
)

type Batcher[T any] struct {
	log       logrus.FieldLogger
	mutex     sync.Mutex
	keyFn     func(T) (string, error)
	combineFn func(T, T) T
	items     map[string]T
}

func DefaultBatcher(log logrus.FieldLogger) *Batcher[*Item] {
	return NewBatcher(log, ItemCacheKey, ItemCacheCombine)
}

func NewBatcher[T any](log logrus.FieldLogger, keyFn func(T) (string, error), combineFn func(T, T) T) *Batcher[T] {
	return &Batcher[T]{
		log:       log,
		keyFn:     keyFn,
		combineFn: combineFn,
	}
}

func (b *Batcher[T]) Write(item T) {
	key, err := b.keyFn(item)
	if err != nil {
		b.log.Errorf("item ignored: failed to determine key: %v", err)
		return
	}

	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.items == nil {
		b.items = make(map[string]T)
	}
	if prev, ok := b.items[key]; ok {
		item = b.combineFn(prev, item)
	}
	b.items[key] = item
}

func (b *Batcher[T]) GetMapAndClear() map[string]T {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if len(b.items) == 0 {
		return nil
	}
	items := b.items
	b.items = nil
	return items
}
