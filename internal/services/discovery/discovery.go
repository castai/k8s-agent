package discovery

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type Service struct {
	clientset kubernetes.Interface
	dyno      dynamic.Interface

	kubeSystemNamespace   *v1.Namespace
	kubeSystemNamespaceMu *sync.Mutex
}

func New(clientset kubernetes.Interface, dyno dynamic.Interface) Service {
	return Service{
		clientset:             clientset,
		dyno:                  dyno,
		kubeSystemNamespaceMu: &sync.Mutex{},
	}
}

// GetCSPAndRegion discovers the cluster cloud service provider (CSP) and the region the cluster is deployed in by
// listing the cluster nodes and inspecting their labels. CSP is retrieved by parsing the Node.Spec.ProviderID property.
// Whereas the region is read from the well-known node region labels.
func (s *Service) GetCSPAndRegion(ctx context.Context) (csp, region *string, reterr error) {
	return s.getCSPAndRegion(ctx, "")
}

func (s *Service) GetClusterID(ctx context.Context) (*uuid.UUID, error) {
	ns, err := s.getKubeSystemNamespace(ctx)
	if err != nil {
		return nil, err
	}

	clusterID, err := uuid.Parse(string(ns.UID))
	if err != nil {
		return nil, fmt.Errorf("parsing namespace %q uid: %w", metav1.NamespaceSystem, err)
	}

	return &clusterID, nil
}

func (s *Service) getKubeSystemNamespace(ctx context.Context) (*v1.Namespace, error) {
	s.kubeSystemNamespaceMu.Lock()
	defer s.kubeSystemNamespaceMu.Unlock()

	if s.kubeSystemNamespace != nil {
		return s.kubeSystemNamespace, nil
	}

	ns, err := s.clientset.CoreV1().Namespaces().Get(ctx, metav1.NamespaceSystem, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting namespace %q: %w", metav1.NamespaceSystem, err)
	}

	s.kubeSystemNamespace = ns

	return ns, nil
}

func (s *Service) getCSPAndRegion(ctx context.Context, next string) (csp, region *string, reterr error) {
	nodes, err := s.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 10, Continue: next})
	if err != nil {
		return nil, nil, fmt.Errorf("listing nodes: %w", err)
	}

	for i, n := range nodes.Items {
		ready := false

		for _, cond := range n.Status.Conditions {
			if cond.Type == v1.NodeReady && cond.Status == v1.ConditionTrue {
				ready = true
				break
			}
		}

		if !ready {
			continue
		}

		nodeCSP, ok := getCSP(&nodes.Items[i])
		if ok {
			csp = &nodeCSP
		}

		nodeRegion, ok := getRegion(&nodes.Items[i])
		if ok {
			region = &nodeRegion
		}

		if csp != nil && region != nil {
			return csp, region, nil
		}
	}

	if nodes.Continue != "" {
		return s.getCSPAndRegion(ctx, nodes.Continue)
	}

	var properties []string
	if csp == nil {
		properties = append(properties, "csp")
	}
	if region == nil {
		properties = append(properties, "region")
	}

	return nil, nil, fmt.Errorf("failed discovering properties: %s", strings.Join(properties, ", "))
}

func getRegion(n *v1.Node) (string, bool) {
	if val, ok := n.Labels[v1.LabelTopologyRegion]; ok {
		return val, true
	}

	if val, ok := n.Labels[v1.LabelFailureDomainBetaRegion]; ok {
		return val, true
	}

	return "", false
}

func getCSP(n *v1.Node) (string, bool) {
	providerID := n.Spec.ProviderID

	if strings.HasPrefix(providerID, "gce://") {
		return "gcp", true
	}

	if strings.HasPrefix(providerID, "aws://") {
		return "aws", true
	}

	return "", false
}
