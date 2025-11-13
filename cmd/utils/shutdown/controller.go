package shutdown

import (
	"sync"

	"github.com/sirupsen/logrus"
)

type Trigger func(error)

type Controller interface {
	For(component string) Trigger
}

type controller struct {
	m        sync.Mutex
	cancelFn func()
	logger   logrus.FieldLogger
}

func (c *controller) For(component string) Trigger {
	if component == "" {
		component = "unknown"
	}
	return func(err error) {
		c.m.Lock()
		defer c.m.Unlock()
		if c.cancelFn == nil {
			// We expect all components to eventually trigger a shutdown as they stop themselves. We only really care if
			// they stop uncleanly.
			if err != nil {
				c.logger.WithError(err).Infof("shutdown already triggered, but %q reported an error", component)
			}
			return
		}
		if err == nil {
			c.logger.Infof("shutdown triggered by %q (no error)", component)
		} else {
			c.logger.WithError(err).Infof("shutdown triggered by %q", component)
		}
		c.cancelFn()
		c.cancelFn = nil
	}
}
