package monitor

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
)

const (
	// CheckInterval controls how often the monitor checks will be performed
	CheckInterval = time.Second
)

func Run(ctx context.Context, log logrus.FieldLogger, clientset *kubernetes.Clientset, metadataFile string, exitCh chan error, clusterIDHandler func(clusterID string)) error {
	m := monitor{
		clientset: clientset,
		log:       log,
	}

	metadataUpdates := make(chan Metadata)
	go watchForMetadataChanges(ctx, metadataFile, m.log, metadataUpdates, exitCh)

	for {
		select {
		case <-ctx.Done():
			return nil
		case metadata := <-metadataUpdates:
			clusterIDHandler(metadata.ClusterID)
			m.metadataUpdated(ctx, metadata)
		}
	}
}

type monitor struct {
	clientset      *kubernetes.Clientset
	log            logrus.FieldLogger
	metadata       Metadata
	agentStartTime int64
}

func (m *monitor) metadataUpdated(ctx context.Context, metadata Metadata) {
	if m.metadata.ProcessID != 0 && m.metadata.ProcessID != metadata.ProcessID {
		m.log.Errorf("unexpected agent restart detected")
		// TODO: fetch and log k8s events for agent process
	}
	m.metadata = metadata

}
