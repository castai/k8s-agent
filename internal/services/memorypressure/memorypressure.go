package memorypressure

import (
	"context"
	"runtime"
	runtimedebug "runtime/debug"
	"time"

	"github.com/sirupsen/logrus"
)

var memoryLimit int64

func init() {
	memoryLimit = runtimedebug.SetMemoryLimit(-1)
}

type MemoryPressure struct {
	Ctx      context.Context
	Interval time.Duration
	Log      logrus.FieldLogger
}

// OnMemoryPressure polls memory usage every Interval and if it reaches
// the GOMEMLIMIT, it will execute f.
// This function is intended to be executed on a separate goroutine.
func (mp *MemoryPressure) OnMemoryPressure(f func()) {
	if mp.Interval <= 0 {
		return
	}
	ticker := time.NewTicker(mp.Interval)
	var m runtime.MemStats
	for {
		select {
		case <-mp.Ctx.Done():
			return
		case <-ticker.C:
			runtime.ReadMemStats(&m)
			memInUse := m.HeapAlloc + m.StackSys + m.OtherSys
			if memInUse >= uint64(memoryLimit) {
				mp.Log.WithFields(logrus.Fields{
					"mem_in_use": memInUse,
					"mem_limit":  memoryLimit,
				}).Info("memory preassure detected, executing callback")
				f()
			}
		}
	}
}
