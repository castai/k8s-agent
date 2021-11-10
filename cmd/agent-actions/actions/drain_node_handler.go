package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"

	"castai-agent/cmd/agent-actions/telemetry"
)

var (
	errPodPresent = errors.New("pod is still present")
)

type drainNodeConfig struct {
	podsDeleteTimeout   time.Duration
	podDeleteRetries    uint64
	podDeleteRetryDelay time.Duration
	podEvictRetryDelay  time.Duration
}

func newDrainNodeHandler(log logrus.FieldLogger, clientset kubernetes.Interface) ActionHandler {
	return &drainNodeHandler{
		log:       log,
		clientset: clientset,
		cfg: drainNodeConfig{
			podsDeleteTimeout:   5 * time.Minute,
			podDeleteRetries:    5,
			podDeleteRetryDelay: 5 * time.Second,
			podEvictRetryDelay:  5 * time.Second,
		},
	}
}

type drainNodeHandler struct {
	log       logrus.FieldLogger
	clientset kubernetes.Interface
	cfg       drainNodeConfig
}

func (h *drainNodeHandler) Handle(ctx context.Context, data []byte) error {
	var req telemetry.AgentActionDrainNode
	if err := json.Unmarshal(data, &req); err != nil {
		return err
	}

	log := h.log.WithField("node_name", req.NodeName)

	node, err := h.clientset.CoreV1().Nodes().Get(ctx, req.NodeName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("node not found, skipping draining")
			return nil
		}
		return err
	}

	log.Infof("draining node")

	if err := h.taintNode(ctx, node); err != nil {
		return fmt.Errorf("tainting node %q: %w", req.NodeName, err)
	}

	pods, err := h.listNodePods(ctx, node)
	if err != nil {
		return fmt.Errorf("listing pods for node %q: %w", req.NodeName, err)
	}

	// First try to evict pods gracefully.
	evictCtx, evictCancel := context.WithTimeout(ctx, time.Duration(req.DrainTimeoutSeconds)*time.Second)
	defer evictCancel()
	err = h.evictPods(evictCtx, log, pods)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	if errors.Is(err, context.DeadlineExceeded) {
		if !req.Force {
			return err
		}
		// If force is set and evict timeout exceeded delete pods.
		deleteCtx, deleteCancel := context.WithTimeout(ctx, h.cfg.podsDeleteTimeout)
		defer deleteCancel()
		if err := h.deletePods(deleteCtx, log, pods.Items); err != nil {
			return err
		}
	}

	log.Info("node drained")

	return nil
}

func (h *drainNodeHandler) deletePods(ctx context.Context, log logrus.FieldLogger, pods []v1.Pod) error {
	log.Infof("forcefully deleting %d pods", len(pods))

	g, ctx := errgroup.WithContext(ctx)
	for _, pod := range pods {
		pod := pod

		if daemonSetPod(pod) {
			continue
		}

		g.Go(func() error {
			err := h.deletePod(ctx, pod)
			if err != nil {
				return err
			}
			return h.waitPodTerminated(ctx, log, pod)
		})
	}

	if err := g.Wait(); err != nil {
		return fmt.Errorf("deleting pods: %w", err)
	}

	return nil
}

func (h *drainNodeHandler) deletePod(ctx context.Context, pod v1.Pod) error {
	b := backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(h.cfg.podDeleteRetryDelay), h.cfg.podDeleteRetries), ctx) // nolint:gomnd
	action := func() error {
		err := h.clientset.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{})
		if err != nil {
			// Pod is not found - ignore.
			if apierrors.IsNotFound(err) {
				return nil
			}

			// Pod is misconfigured - stop retry.
			if apierrors.IsInternalError(err) {
				return backoff.Permanent(err)
			}
		}

		// Other errors - retry.
		return err
	}
	if err := backoff.Retry(action, b); err != nil {
		return fmt.Errorf("deleting pod %s in namespace %s: %w", pod.Name, pod.Namespace, err)
	}
	return nil
}

// taintNode to make it unshedulable.
func (h *drainNodeHandler) taintNode(ctx context.Context, node *v1.Node) error {
	if node.Spec.Unschedulable {
		return nil
	}

	err := patchNode(ctx, h.clientset, node, func(n *v1.Node) error {
		n.Spec.Unschedulable = true
		return nil
	})
	if err != nil {
		return fmt.Errorf("patching node unschedulable: %w", err)
	}
	return nil
}

// listNodePods returns a list of all pods scheduled on the provided node.
func (h *drainNodeHandler) listNodePods(ctx context.Context, node *v1.Node) (*v1.PodList, error) {
	var pods *v1.PodList
	err := backoff.Retry(func() error {
		p, err := h.clientset.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
			FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": node.Name}).String(),
		})
		if err != nil {
			return err
		}
		pods = p
		return nil
	}, defaultBackoff(ctx))
	return pods, err
}

func (h *drainNodeHandler) evictPods(ctx context.Context, log logrus.FieldLogger, pods *v1.PodList) error {
	log.Infof("evicting %d pods", len(pods.Items))

	g, ctx := errgroup.WithContext(ctx)
	for _, pod := range pods.Items {
		pod := pod

		if daemonSetPod(pod) {
			continue
		}

		g.Go(func() error {
			err := h.evictPod(ctx, pod)
			if err != nil {
				return err
			}
			return h.waitPodTerminated(ctx, log, pod)
		})
	}

	if err := g.Wait(); err != nil {
		return fmt.Errorf("evicting pods: %w", err)
	}

	return nil
}

func (h *drainNodeHandler) waitPodTerminated(ctx context.Context, log logrus.FieldLogger, pod v1.Pod) error {
	b := backoff.WithContext(backoff.NewConstantBackOff(5*time.Second), ctx) // nolint:gomnd

	err := backoff.Retry(func() error {
		p, err := h.clientset.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil && apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		// replicaSets will recreate pods with equal name and namespace, therefore we compare UIDs
		if p.GetUID() == pod.GetUID() {
			return errPodPresent
		}
		return nil
	}, b)
	if err != nil && errors.Is(err, errPodPresent) {
		log.Infof("timeout waiting for pod %s in namespace %s to terminate", pod.Name, pod.Namespace)
		return nil
	}
	if err != nil {
		return fmt.Errorf("waiting for pod %s in namespace %s termination: %w", pod.Name, pod.Namespace, err)
	}
	return nil
}

// evictPod from the k8s node. Error handling is based on eviction api documentation:
// https://kubernetes.io/docs/tasks/administer-cluster/safely-drain-node/#the-eviction-api
func (h *drainNodeHandler) evictPod(ctx context.Context, pod v1.Pod) error {
	b := backoff.WithContext(backoff.NewConstantBackOff(h.cfg.podEvictRetryDelay), ctx) // nolint:gomnd
	action := func() error {
		err := h.clientset.CoreV1().Pods(pod.Namespace).Evict(ctx, &v1beta1.Eviction{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "policy/v1beta1",
				Kind:       "Eviction",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
		})

		if err != nil {
			// Pod is not found - ignore.
			if apierrors.IsNotFound(err) {
				return nil
			}

			// Pod is misconfigured - stop retry.
			if apierrors.IsInternalError(err) {
				return backoff.Permanent(err)
			}
		}

		// Other errors - retry.
		return err
	}
	if err := backoff.Retry(action, b); err != nil {
		return fmt.Errorf("evicting pod %s in namespace %s: %w", pod.Name, pod.Namespace, err)
	}
	return nil
}

func daemonSetPod(pod v1.Pod) bool {
	controller := metav1.GetControllerOf(&pod)
	return controller != nil && controller.Kind == "DaemonSet"
}
