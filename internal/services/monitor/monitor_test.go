package monitor

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func Test_monitor_waitForClusterID(t *testing.T) {
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
	require.NoError(t, monitor.waitForClusterID(ctx))
}
