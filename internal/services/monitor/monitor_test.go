package monitor

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"

	mock_monitor "castai-agent/internal/services/monitor/mock"
)

func Test_monitor_waitForAgentMetadata(t *testing.T) {
	monitor := monitor{
		log: logrus.New(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	syncFile := filepath.Join(t.TempDir(), "metadata.yaml")

	monitor.syncFile = syncFile
	go func() {
		time.Sleep(time.Second * 1)

		meta := Metadata{
			ClusterID: uuid.New().String(),
			ProcessID: 123,
		}
		require.NoError(t, meta.Save(syncFile))
	}()
	require.NoError(t, monitor.waitForAgentMetadata(ctx))
}

func Test_monitor_runChecks(t *testing.T) {
	testLog, hook := test.NewNullLogger()
	monitor := monitor{
		log:            testLog,
		agentStartTime: 1,
	}
	monitor.metadata.ProcessID = 123
	r := require.New(t)

	ctrl := gomock.NewController(t)
	processInfo := mock_monitor.NewMockProcessInfo(ctrl)
	processInfo.EXPECT().GetProcessStartTime().Return(uint64(2))
	monitor.processInfo = processInfo

	r.NoError(monitor.runChecks(context.Background()))
	r.Len(hook.Entries, 1)
	r.Equal("unexpected agent restart detected", hook.Entries[0].Message)
}
