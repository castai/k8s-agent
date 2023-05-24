package monitor

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
)

func Run(ctx context.Context, log logrus.FieldLogger, clientset *kubernetes.Clientset, metadataFile string, clusterIDHandler func(clusterID string)) error {
	m := monitor{
		clientset: clientset,
		log:       log,
	}

	metadataUpdates, err := watchForMetadataChanges(ctx, metadataFile, m.log)
	if err != nil {
		return fmt.Errorf("setting up metadata watch: %w", err)
	}

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
