//go:generate mockgen -source $GOFILE -destination ./mock/discovery.go . Service
package discovery

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"castai-agent/pkg/cloud"
)

type Service interface {
	// GetCSPAndRegion discovers the cluster cloud service provider (CSP) and the region the cluster is deployed in by
	// listing the cluster nodes and inspecting their labels. CSP is retrieved by parsing the Node.Spec.ProviderID property.
	// Whereas the region is read from the well-known node region labels.
	GetCSPAndRegion(ctx context.Context) (csp cloud.Cloud, region string, reterr error)

	// GetClusterID retrieves the cluster ID by reading the UID of the kube-system namespace.
	GetClusterID(ctx context.Context) (*uuid.UUID, error)

	// GetKOPSClusterNameAndStateStore discovers the cluster name and kOps state store bucket from the kube-system namespace
	// annotation. kOps annotates the kube-system namespace with annotations such as this:
	// * addons.k8s.io/core.addons.k8s.io: '{"version":"1.4.0","channel":"s3://bucket/cluster-name/addons/bootstrap-channel.yaml","manifestHash":"hash"}'
	// We can retrieve the state store bucket name and the cluster name from the "channel" property of the annotation value.
	GetKOPSClusterNameAndStateStore(ctx context.Context, log logrus.FieldLogger) (clusterName, stateStore string, reterr error)

	// GetOpenshiftClusterID discovers the OpenShift cluster ID by reading the cluster ID from the OpenShift cluster version resource.
	GetOpenshiftClusterID(ctx context.Context) (string, error)

	// GetOpenshiftClusterName discovers the OpenShift cluster name by reading the cluster name from the OpenShift master machine resource.
	GetOpenshiftClusterName(ctx context.Context) (string, error)
}

var _ Service = (*ServiceImpl)(nil)

type ServiceImpl struct {
	clientset kubernetes.Interface
	dyno      dynamic.Interface

	kubeSystemNamespace   *v1.Namespace
	kubeSystemNamespaceMu *sync.Mutex
}

func New(clientset kubernetes.Interface, dyno dynamic.Interface) *ServiceImpl {
	return &ServiceImpl{
		clientset:             clientset,
		dyno:                  dyno,
		kubeSystemNamespaceMu: &sync.Mutex{},
	}
}

func (s *ServiceImpl) GetCSPAndRegion(ctx context.Context) (csp cloud.Cloud, region string, reterr error) {
	return s.getCSPAndRegion(ctx, "")
}

func (s *ServiceImpl) GetClusterID(ctx context.Context) (*uuid.UUID, error) {
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

func (s *ServiceImpl) getKubeSystemNamespace(ctx context.Context) (*v1.Namespace, error) {
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

func (s *ServiceImpl) getCSPAndRegion(ctx context.Context, next string) (csp cloud.Cloud, region string, reterr error) {
	nodes, err := s.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 10, Continue: next})
	if err != nil {
		return "", "", fmt.Errorf("listing nodes: %w", err)
	}

	for i := range nodes.Items {
		node := &nodes.Items[i]

		if !isNodeReady(node) {
			continue
		}

		nodeCSP, ok := getCSP(node)
		if ok {
			csp = nodeCSP
		}

		nodeRegion, ok := getRegion(node)
		if ok {
			region = nodeRegion
		}

		if csp != "" && region != "" {
			return csp, region, nil
		}
	}

	if nodes.Continue != "" {
		return s.getCSPAndRegion(ctx, nodes.Continue)
	}

	return "", "", fmt.Errorf("failed discovering properties: csp=%q, region=%q", csp, region)
}

func isNodeReady(n *v1.Node) bool {
	for _, cond := range n.Status.Conditions {
		if cond.Type == v1.NodeReady && cond.Status == v1.ConditionTrue {
			return true
		}
	}

	return false
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

func getCSP(n *v1.Node) (cloud.Cloud, bool) {
	providerID := n.Spec.ProviderID

	if strings.HasPrefix(providerID, "gce://") {
		return cloud.GCP, true
	}

	if strings.HasPrefix(providerID, "aws://") {
		return cloud.AWS, true
	}

	return "", false
}
