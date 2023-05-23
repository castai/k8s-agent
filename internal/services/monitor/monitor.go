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

	if err := m.waitForAgentMetadata(ctx); err != nil {
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
	clientset      *kubernetes.Clientset
	log            logrus.FieldLogger
	syncFile       string
	metadata       Metadata
	processInfo    ProcessInfo
	agentStartTime int64
}

// waitForAgentMetadata waits until main agent process registers itself with CAST AI, and shares a metadata file on a local volume
func (m *monitor) waitForAgentMetadata(ctx context.Context) (err error) {
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
		metadata := Metadata{}
		if err := metadata.Load(m.syncFile); err != nil {
			m.log.Warnf("loading metadata failed: %v", err)
		} else {
			m.metadata = metadata
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

func (m *monitor) runChecks(ctx context.Context) error {
	agentStartTime, err := m.processInfo.GetProcessStartTime(ctx)
	if err != nil {
		m.log.Errorf("checks failed: %v", err)
	}
	if m.agentStartTime != 0 && agentStartTime > m.agentStartTime {
		m.log.Errorf("unexpected agent restart detected")
		// TODO: fetch and log k8s events for agent process
	}
	m.agentStartTime = agentStartTime
	return nil
}
