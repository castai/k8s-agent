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
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	syncFile := filepath.Join(t.TempDir(), "metadata.json")

	updates, err := watchForMetadataChanges(ctx, syncFile, logrus.New())
	require.NoError(t, err)

	// make sure that watcher does not find the file immediately and goes into watcher loop
	time.Sleep(time.Second * 1)

	// create the file, expect the event to arrive at updates channel
	meta := Metadata{
		ClusterID: uuid.New().String(),
		ProcessID: 123,
	}
	require.NoError(t, meta.Save(syncFile))

	metadata, ok := <-updates
	require.True(t, ok)
	require.Equal(t, uint64(123), metadata.ProcessID)

	cancel()
	_, ok = <-updates
	require.False(t, ok, "after ctx is done, updates channel should get closed as watcher exits")
}
