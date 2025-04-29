package delta

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"castai-agent/internal/castai"
)

func DefaultCompiler(log logrus.FieldLogger, clusterID, clusterVersion, agentVersion string) *Compiler[*Item] {
	return NewCompiler(time.Now, log, clusterID, clusterVersion, agentVersion, ItemCacheCompile)
}

func NewCompiler[T any](
	timeNow func() time.Time,
	log logrus.FieldLogger,
	clusterID, clusterVersion, agentVersion string,
	compileItemFn func(T) (*castai.DeltaItem, error),
) *Compiler[T] {
	return &Compiler[T]{
		timeNow:        timeNow,
		log:            log,
		clusterID:      clusterID,
		clusterVersion: clusterVersion,
		agentVersion:   agentVersion,
		compileItemFn:  compileItemFn,
	}
}

type Compiler[T any] struct {
	timeNow        func() time.Time
	log            logrus.FieldLogger
	clusterID      string
	clusterVersion string
	agentVersion   string

	mutex         sync.Mutex
	items         map[string]*castai.DeltaItem
	compileItemFn func(T) (*castai.DeltaItem, error)
}

func (c *Compiler[T]) Write(items map[string]T) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.items == nil {
		c.items = make(map[string]*castai.DeltaItem)
	}

	// Caching the value to avoid repeated system calls. Also aligns items computed for the same iteration.
	now := c.timeNow().UTC()

	// Refreshing previously compiled items since they haven't been uploaded and the fact that we don't re-compile them
	// is just an implementation-specific optimisation.
	for _, value := range c.items {
		value.CreatedAt = now
	}

	for key, itemRaw := range items {
		value, err := c.compileItemFn(itemRaw)
		if err != nil {
			c.log.Errorf("failed to compile delta item: %v", err)
			continue
		}
		value.CreatedAt = now
		// Note: potentially overwriting previously compiled items as means to deduplicate them in case they hadn't been
		// successfully uploaded and them changing in the meantime.
		c.items[key] = value
	}
}

func (c *Compiler[T]) AsCastDelta(fullSnapshot bool) *castai.Delta {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	var deltaItems = make([]*castai.DeltaItem, 0, len(c.items))
	for _, value := range c.items {
		deltaItems = append(deltaItems, value)
	}

	delta := &castai.Delta{
		ClusterID:      c.clusterID,
		ClusterVersion: c.clusterVersion,
		AgentVersion:   c.agentVersion,
		FullSnapshot:   fullSnapshot,
		Items:          deltaItems,
	}
	return delta
}

func (c *Compiler[T]) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.items = nil
}
