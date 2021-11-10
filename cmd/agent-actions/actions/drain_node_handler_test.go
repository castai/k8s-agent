package actions

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktest "k8s.io/client-go/testing"

	"castai-agent/cmd/agent-actions/telemetry"
)

func TestDrainNodeHandler(t *testing.T) {
	r := require.New(t)

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	t.Run("drain successfully", func(t *testing.T) {
		nodeName := "node1"
		podName := "pod1"
		clientset := setupFakeClientWithNodePodEviction(nodeName, podName)
		prependEvictionReaction(t, clientset, true)

		h := drainNodeHandler{
			log:       log,
			clientset: clientset,
			cfg:       drainNodeConfig{},
		}

		req := telemetry.AgentActionDrainNode{
			NodeName:            "node1",
			DrainTimeoutSeconds: 1,
			Force:               true,
		}
		reqData, _ := json.Marshal(req)

		err := h.Handle(context.Background(), reqData)
		r.NoError(err)

		n, err := clientset.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
		r.NoError(err)
		r.True(n.Spec.Unschedulable)

		_, err = clientset.CoreV1().Pods("default").Get(context.Background(), podName, metav1.GetOptions{})
		r.Error(err)
		r.True(apierrors.IsNotFound(err))
	})

	t.Run("skip drain when node not found", func(t *testing.T) {
		nodeName := "node1"
		podName := "pod1"
		clientset := setupFakeClientWithNodePodEviction(nodeName, podName)
		prependEvictionReaction(t, clientset, true)

		h := drainNodeHandler{
			log:       log,
			clientset: clientset,
			cfg:       drainNodeConfig{},
		}

		req := telemetry.AgentActionDrainNode{
			NodeName:            "already-deleted-node",
			DrainTimeoutSeconds: 1,
			Force:               true,
		}
		reqData, _ := json.Marshal(req)

		err := h.Handle(context.Background(), reqData)
		r.NoError(err)

		_, err = clientset.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
		r.NoError(err)
	})

	t.Run("fail to drain when internal pod eviction error occur", func(t *testing.T) {
		nodeName := "node1"
		podName := "pod1"
		clientset := setupFakeClientWithNodePodEviction(nodeName, podName)
		prependEvictionReaction(t, clientset, false)

		h := drainNodeHandler{
			log:       log,
			clientset: clientset,
			cfg:       drainNodeConfig{},
		}

		req := telemetry.AgentActionDrainNode{
			NodeName:            "node1",
			DrainTimeoutSeconds: 1,
			Force:               true,
		}
		reqData, _ := json.Marshal(req)

		err := h.Handle(context.Background(), reqData)
		r.EqualError(err, "evicting pods: evicting pod pod1 in namespace default: internal")

		n, err := clientset.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
		r.NoError(err)
		r.True(n.Spec.Unschedulable)

		_, err = clientset.CoreV1().Pods("default").Get(context.Background(), podName, metav1.GetOptions{})
		r.NoError(err)
	})
}

func prependEvictionReaction(t *testing.T, c *fake.Clientset, success bool) {
	c.PrependReactor("create", "pods", func(action ktest.Action) (handled bool, ret runtime.Object, err error) {
		if action.GetSubresource() != "eviction" {
			return false, nil, nil
		}

		if success {
			eviction := action.(ktest.CreateAction).GetObject().(*v1beta1.Eviction)

			go func() {
				err := c.CoreV1().Pods(eviction.Namespace).Delete(context.Background(), eviction.Name, metav1.DeleteOptions{})
				require.NoError(t, err)
			}()

			return true, nil, nil
		}

		return true, nil, &apierrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonInternalError, Message: "internal"}}
	})
}

func setupFakeClientWithNodePodEviction(nodeName, podName string) *fake.Clientset {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
		},
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: "default",
		},
		Spec: v1.PodSpec{
			NodeName: nodeName,
		},
	}
	controller := true
	daemonSetPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dsPod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "DaemonSet",
					Controller: &controller,
				},
			},
		},
		Spec: v1.PodSpec{
			NodeName: nodeName,
		},
	}

	clientset := fake.NewSimpleClientset(node, pod, daemonSetPod)

	addEvictionSupport(clientset)

	return clientset
}

func addEvictionSupport(c *fake.Clientset) {
	podsEviction := metav1.APIResource{
		Name:    "pods/eviction",
		Kind:    "Eviction",
		Group:   "",
		Version: "v1",
	}
	coreResources := &metav1.APIResourceList{
		GroupVersion: "v1",
		APIResources: []metav1.APIResource{podsEviction},
	}

	c.Resources = append(c.Resources, coreResources)
}
