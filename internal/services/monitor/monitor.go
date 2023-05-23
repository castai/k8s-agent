package monitor

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
)

func Run(ctx context.Context, log logrus.FieldLogger, clientset *kubernetes.Clientset, clusterIDHandler func(clusterID string)) error {
	m := monitor{
		clientset: clientset,
		log:       log,
	}

	if err := m.waitForClusterID(ctx); err != nil {
		return fmt.Errorf("waiting for cluster ID: %w", err)
	}

	clusterIDHandler(m.metadata.ClusterID)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second * 5):
		}

		if err := m.runChecks(ctx); err != nil {
			return fmt.Errorf("running monitor checks: %w", err)
		}

		_ = clientset
	}
}

type monitor struct {
	clientset *kubernetes.Clientset
	log       logrus.FieldLogger
	syncFile  string
	metadata  Metadata
}

func (m *monitor) waitForClusterID(ctx context.Context) (err error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	if err := watcher.Add(filepath.Dir(m.syncFile)); err != nil {
		return fmt.Errorf("adding watch: %w", err)
	}

	for {
		// try loading the file on startup and on every file system change
		if err := m.loadClusterIDMetadata(); err != nil {
			m.log.Warnf("loading metadata failed: %v", err)
		} else {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case _ = <-watcher.Events:
		case err := <-watcher.Errors:
			m.log.Errorf("error: %v", err)
		}
	}

	return nil
}

func (m *monitor) runChecks(_ context.Context) error {
	m.log.Infof("running checks")
	return nil
}

func (m *monitor) loadClusterIDMetadata() error {
	metadata := Metadata{}
	if err := metadata.Load(m.syncFile); err != nil {
		return fmt.Errorf("parsing json: %w", err)
	}
	m.metadata = metadata

	return nil
}
