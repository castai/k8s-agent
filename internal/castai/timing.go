package castai

import "time"

type Timer struct {
	startedAt time.Time
	stoppedAt time.Time
}

func NewTimer() *Timer {
	return &Timer{
		startedAt: time.Now(),
	}
}

func (t *Timer) Stop() {
	t.stoppedAt = time.Now()
}

func (t *Timer) Duration() time.Duration {
	return t.stoppedAt.Sub(t.startedAt)
}
