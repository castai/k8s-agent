package monitor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
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

	metadataUpdates, err := watchForMetadataChanges(ctx, m.log, metadataFile)
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

	m.reportPodDiagnostics(ctx, prevMetadata.ProcessID)
}

func (m *monitor) reportPodDiagnostics(ctx context.Context, prevProcessPID uint64) {
	m.log.Errorf("unexpected agent restart detected, fetching k8s events for %s/%s", m.pod.Namespace, m.pod.Name)

	// log pod-related warnings
	m.logEvents(ctx, m.log.WithField("events_group", fmt.Sprintf("%s/%s", m.pod.Namespace, m.pod.Name)), m.pod.Namespace, &metav1.ListOptions{
		FieldSelector: "involvedObject.name=" + m.pod.Name,
		TypeMeta: metav1.TypeMeta{
			Kind: "Pod",
		},
	}, func(event *v1.Event) bool {
		return true
	})

	// Log node-related warnings. We can't find relevant messages easily as there's no metadata linking events to specific pods,
	// and even filtering by PID id does not work (agent process PID is different inside the pod and as seen from the node).
	// Instead, will use simple filtering by "castai-agent"; combined with node-name filter, this should be sufficient enough
	// to narrow the list down to agent-related events only.
	m.logEvents(ctx, m.log.WithFields(logrus.Fields{
		"events_group": fmt.Sprintf("node/%s", m.pod.Node),
		"pid":          prevProcessPID,
	}), v1.NamespaceAll, &metav1.ListOptions{
		FieldSelector: "involvedObject.name=" + m.pod.Node,
		TypeMeta: metav1.TypeMeta{
			Kind: "Node",
		},
	}, func(event *v1.Event) bool {
		// OOM events are reported on the node, but the only relation to the pod is the killed process PID.
		return strings.Contains(event.Message, "castai-agent")
	})
}

func (m *monitor) logEvents(ctx context.Context, log logrus.FieldLogger, namespace string, listOptions *metav1.ListOptions, filter func(event *v1.Event) bool) {
	events, err := m.clientset.CoreV1().Events(namespace).List(ctx, *listOptions)
	if err != nil {
		log.Errorf("failed fetching k8s events after agent restart: %v", err)
		return
	}
	relevantEvents := lo.Filter(events.Items, func(e v1.Event, _ int) bool {
		return e.Type != v1.EventTypeNormal && filter(&e)
	})

	if len(relevantEvents) == 0 {
		log.Warnf("no relevant k8s events detected out of %d retrieved", len(events.Items))
		return
	}

	for _, e := range relevantEvents {
		log.Errorf("k8s events detected: TYPE:%s REASON:%s TIMESTAMP:%s MESSAGE:%s", e.Type, e.Reason, e.LastTimestamp.UTC().Format(time.RFC3339), e.Message)
	}
}
