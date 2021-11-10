package actions

import (
	"context"
	"encoding/json"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"castai-agent/cmd/agent-actions/telemetry"
)

func newPatchNodeHandler(log logrus.FieldLogger, clientset kubernetes.Interface) ActionHandler {
	return &patchNodeHandler{
		log:       log,
		clientset: clientset,
	}
}

type patchNodeHandler struct {
	log       logrus.FieldLogger
	clientset kubernetes.Interface
}

func (h *patchNodeHandler) Handle(ctx context.Context, data []byte) error {
	var req telemetry.AgentActionPatchNode
	if err := json.Unmarshal(data, &req); err != nil {
		return err
	}

	log := h.log.WithField("node_name", req.NodeName)

	node, err := h.clientset.CoreV1().Nodes().Get(ctx, req.NodeName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("node not found, skipping patch")
			return nil
		}
		return err
	}

	return patchNode(ctx, h.clientset, node, func(n *v1.Node) error {
		if n.Labels == nil {
			n.Labels = map[string]string{}
		}

		for key, val := range req.Labels {
			n.Labels[key] = val
		}

		seen := make(map[string]struct{}, len(n.Spec.Taints))
		for _, taint := range n.Spec.Taints {
			seen[taint.Key] = struct{}{}
		}

		for _, newTaint := range req.Taints {
			if _, ok := seen[newTaint.Key]; ok {
				continue
			}
			taint := v1.Taint{
				Key:    newTaint.Key,
				Value:  newTaint.Value,
				Effect: v1.TaintEffect(newTaint.Effect),
			}
			n.Spec.Taints = append(n.Spec.Taints, taint)
		}

		return nil
	})
}
