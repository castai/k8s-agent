package monitor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestSaveMetadata(t *testing.T) {

	tests := map[string]struct {
		createDir     string
		file          string
		expectedError *string
	}{
		"not configured": {
			file:          "",
			expectedError: nil,
		},
		"invalid file dir": {
			file:          "no_such_dir/abc",
			expectedError: lo.ToPtr("open.*no such file or directory"),
		},
		"valid dir": {
			createDir: "metadata",
			file:      "metadata/info",
		},
	}

	for testName, tt := range tests {
		tt := tt
		t.Run(testName, func(t *testing.T) {
			r := require.New(t)
			baseDir := t.TempDir()
			if tt.createDir != "" {
				r.NoError(os.MkdirAll(filepath.Join(baseDir, tt.createDir), 0700))
			}
			m := Metadata{
				ClusterID: uuid.New().String(),
				ProcessID: 123,
			}
			saveTo := tt.file
			if tt.file != "" {
				saveTo = filepath.Join(baseDir, tt.file)
			}

			err := m.Save(saveTo)
			if tt.expectedError == nil {
				r.NoError(err)
			} else {
				r.Regexp(*tt.expectedError, err.Error())
			}
		})
	}
}

func Test_monitor_waitForAgentMetadata(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	syncFile := filepath.Join(t.TempDir(), "metadata.json")

	updates, err := watchForMetadataChanges(ctx, logrus.New(), syncFile)
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

	for range updates {
		// exhaust other events
	}
	_, ok = <-updates
	require.False(t, ok, "after ctx is done, updates channel should get closed as watcher exits")
}
