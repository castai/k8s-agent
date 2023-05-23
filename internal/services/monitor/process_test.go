package monitor

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_processInfo_GetProcessStartTime(t *testing.T) {
	info := newProcessInfo(int32(os.Getpid()))
	startTime, err := info.GetProcessStartTime(context.Background())
	r := require.New(t)

	r.NoError(err)
	r.Greater(startTime, int64(0))
}
