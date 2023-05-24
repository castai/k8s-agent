package monitor

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"castai-agent/internal/config"
)

func Run(ctx context.Context, log logrus.FieldLogger, clientset *kubernetes.Clientset, metadataFile string, pod config.Pod, clusterIDHandler func(clusterID string)) error {
	m := monitor{
		clientset: clientset,
		log:       log,
		pod:       pod,
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
	clientset *kubernetes.Clientset
	log       logrus.FieldLogger
	metadata  Metadata
	pod       config.Pod
}

// metadataUpdated gets called each time we receive a notification from metadata file watcher that there were changes to it
func (m *monitor) metadataUpdated(ctx context.Context, metadata Metadata) {
	prevMetadata := m.metadata
	m.metadata = metadata
	if prevMetadata.ProcessID == 0 || prevMetadata.ProcessID == metadata.ProcessID {
		// if we just received first metadata or there were no changes, nothing to do
		return
	}

	m.reportPodDiagnostics(ctx)
}

func (m *monitor) reportPodDiagnostics(ctx context.Context) {
	m.log.Errorf("unexpected agent restart detected, fetching k8s events for %s/%s", m.pod.Namespace, m.pod.Name)

	events, err := m.clientset.CoreV1().Events(m.pod.Namespace).List(ctx, metav1.ListOptions{
		FieldSelector: "involvedObject.name=" + m.pod.Name,
		TypeMeta: metav1.TypeMeta{
			Kind: "Pod",
		},
	})
	if err != nil {
		m.log.Errorf("failed fetching k8s events after agent restart: %v", err)
	} else {
		if len(events.Items) == 0 {
			m.log.Warnf("no k8s events detected for %s/%s", m.pod.Namespace, m.pod.Name)
		}
		for _, e := range events.Items {
			m.log.Errorf("k8s events detected: TYPE:%s REASON:%s TIMESTAMP:%s MESSAGE:%s", e.Type, e.Reason, e.LastTimestamp.UTC().Format(time.RFC3339), e.Message)
		}
	}
}
