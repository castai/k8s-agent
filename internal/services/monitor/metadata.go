package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
)

type Metadata struct {
	ClusterID string
	ProcessID uint64
}

func (m *Metadata) Save(file string) error {
	contents, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshaling: %w", err)
	}
	return os.WriteFile(file, contents, 0600)
}

func (m *Metadata) Load(file string) error {
	contents, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	if err := json.Unmarshal(contents, m); err != nil {
		return fmt.Errorf("parsing json: %w", err)
	}
	return nil
}

// watchForMetadataChanges watches a local file for updates and returns changes to metadata channel; exits on context cancel
func watchForMetadataChanges(ctx context.Context, metadataFilePath string, log logrus.FieldLogger, updates chan Metadata, exitCh chan error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		exitCh <- fmt.Errorf("setting up new watcher: %w", err)
		return
	}
	defer watcher.Close()

	if err := watcher.Add(filepath.Dir(metadataFilePath)); err != nil {
		exitCh <- fmt.Errorf("adding watch: %w", err)
		return
	}

	for {
		// try loading the file on startup and on every file system change
		metadata := Metadata{}
		if err := metadata.Load(metadataFilePath); err != nil {
			log.Warnf("loading metadata failed: %v", err)
		} else {
			updates <- metadata
		}

		select {
		case <-ctx.Done():
			return
		case _ = <-watcher.Events:
		case err := <-watcher.Errors:
			log.Errorf("metadata watch error: %v", err)
		}
	}
}
