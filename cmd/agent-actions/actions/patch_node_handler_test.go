package actions

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"castai-agent/cmd/agent-actions/telemetry"
)

func TestPatchNodeHandler(t *testing.T) {
	r := require.New(t)

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	nodeName := "node1"
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
		},
	}
	clientset := fake.NewSimpleClientset(node)

	t.Run("patch successfully", func(t *testing.T) {
		h := patchNodeHandler{
			log:       log,
			clientset: clientset,
		}

		req := telemetry.AgentActionPatchNode{
			NodeName: "node1",
			Labels: map[string]string{
				"label": "ok",
			},
			Taints: []telemetry.NodeTaint{
				{
					Key:    "taint",
					Value:  "ok2",
					Effect: "NoSchedule",
				},
			},
		}
		reqData, _ := json.Marshal(req)

		err := h.Handle(context.Background(), reqData)
		r.NoError(err)

		n, err := clientset.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
		r.NoError(err)
		r.Equal("ok", n.Labels["label"])
		r.Equal([]v1.Taint{{Key: "taint", Value: "ok2", Effect: "NoSchedule", TimeAdded: (*metav1.Time)(nil)}}, n.Spec.Taints)
	})

	t.Run("skip patch when node not found", func(t *testing.T) {
		h := patchNodeHandler{
			log:       log,
			clientset: clientset,
		}

		req := telemetry.AgentActionDeleteNode{
			NodeName: "already-deleted-node",
		}
		reqData, _ := json.Marshal(req)

		err := h.Handle(context.Background(), reqData)
		r.NoError(err)

		_, err = clientset.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
		r.NoError(err)
	})
}
