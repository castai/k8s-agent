package actions

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"castai-agent/cmd/agent-actions/telemetry"
)

func TestDeleteNodeHandler(t *testing.T) {
	r := require.New(t)

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	t.Run("delete successfully", func(t *testing.T) {
		nodeName := "node1"
		node := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
			},
		}
		clientset := fake.NewSimpleClientset(node)

		h := deleteNodeHandler{
			log:       log,
			clientset: clientset,
			cfg:       deleteNodeConfig{},
		}

		req := telemetry.AgentActionDeleteNode{
			NodeName: "node1",
		}
		reqData, _ := json.Marshal(req)

		err := h.Handle(context.Background(), reqData)
		r.NoError(err)

		_, err = clientset.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
		r.Error(err)
		r.True(apierrors.IsNotFound(err))
	})

	t.Run("skip delete when node not found", func(t *testing.T) {
		nodeName := "node1"
		node := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
			},
		}
		clientset := fake.NewSimpleClientset(node)

		h := deleteNodeHandler{
			log:       log,
			clientset: clientset,
			cfg:       deleteNodeConfig{},
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
