package shutdown

import (
	"context"

	"github.com/sirupsen/logrus"
)

func Context(parent context.Context, logger logrus.FieldLogger) (context.Context, Controller) {
	ctx, cancelCtx := context.WithCancel(parent)
	return ctx, &controller{
		cancelFn: cancelCtx,
		logger:   logger,
	}
}
