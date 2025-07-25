package castai

import "sync"

// Registrator is used to block until the agent is registered.
type Registrator struct {
	once    sync.Once
	blockCh chan struct{}
}

func NewRegistrator() *Registrator {
	return &Registrator{
		blockCh: make(chan struct{}),
	}
}

// ReleaseWaiters releases all waiters.
func (r *Registrator) ReleaseWaiters() {
	r.once.Do(func() {
		close(r.blockCh)
	})
}

// WaitUntilRegistered blocks until the agent is registered.
func (r *Registrator) WaitUntilRegistered() {
	<-r.blockCh
}
