package actions

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"castai-agent/cmd/agent-actions/telemetry"
)

type deleteNodeConfig struct {
	deleteRetries   uint64
	deleteRetryWait time.Duration
}

func newDeleteNodeHandler(log logrus.FieldLogger, clientset kubernetes.Interface) ActionHandler {
	return &deleteNodeHandler{
		log:       log,
		clientset: clientset,
		cfg: deleteNodeConfig{
			deleteRetries:   5,
			deleteRetryWait: 1 * time.Second,
		},
	}
}

type deleteNodeHandler struct {
	log       logrus.FieldLogger
	clientset kubernetes.Interface
	cfg       deleteNodeConfig
}

func (h *deleteNodeHandler) Handle(ctx context.Context, data []byte) error {
	var req telemetry.AgentActionDeleteNode
	if err := json.Unmarshal(data, &req); err != nil {
		return err
	}
	log := h.log.WithField("node_name", req.NodeName)
	log.Info("deleting kubernetes node")

	node, err := h.clientset.CoreV1().Nodes().Get(ctx, req.NodeName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("node not found, skipping delete")
			return nil
		}
		return err
	}

	b := backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(h.cfg.deleteRetryWait), h.cfg.deleteRetries), ctx)
	return backoff.Retry(func() error {
		return h.clientset.CoreV1().Nodes().Delete(ctx, node.Name, metav1.DeleteOptions{})
	}, b)
}
