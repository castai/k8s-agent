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

func Test_monitor_waitForAgentMetadata(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	syncFile := filepath.Join(t.TempDir(), "metadata.yaml")

	exitCh := make(chan error)
	updates := make(chan Metadata, 1)

	go func() {
		time.Sleep(time.Second * 1)

		meta := Metadata{
			ClusterID: uuid.New().String(),
			ProcessID: 123,
		}
		require.NoError(t, meta.Save(syncFile))

		cancel()
	}()

	// run synchronously to ensure that watch exits after context cancel
	watchForMetadataChanges(ctx, syncFile, logrus.New(), updates, exitCh)

	metadata := <-updates
	require.Equal(t, uint64(123), metadata.ProcessID)
}
