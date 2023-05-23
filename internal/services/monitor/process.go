//go:generate mockgen -source $GOFILE -destination ./mock/$GOFILE .
package monitor

import (
	"context"
	"fmt"

	"github.com/shirou/gopsutil/v3/process"
)

type ProcessInfo interface {
	GetProcessStartTime(ctx context.Context) (int64, error)
}

var _ ProcessInfo = (*processInfo)(nil)

func newProcessInfo(pid int32) *processInfo {
	return &processInfo{pid: pid}
}

type processInfo struct {
	pid int32
}

func (p *processInfo) GetProcessStartTime(ctx context.Context) (int64, error) {
	pp, err := process.NewProcessWithContext(ctx, p.pid)
	if err != nil {
		return 0, fmt.Errorf("resolving process: %w", err)
	}
	createTime, err := pp.CreateTimeWithContext(ctx)
	if err != nil {
		return 0, fmt.Errorf("getting process create time: %w", err)
	}
	return createTime, nil
}
