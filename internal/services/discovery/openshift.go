package discovery

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	OpenshiftMachineAPINamespace = "openshift-machine-api"

	OpenshiftMachineRoleLabel = "machine.openshift.io/cluster-api-machine-role"
	OpenshiftClusterNameLabel = "machine.openshift.io/cluster-api-cluster"

	OpenshiftMasterMachineRole = "master"
)

var (
	OpenshiftMachinesGVR = schema.GroupVersionResource{
		Group:    "machine.openshift.io",
		Version:  "v1beta1",
		Resource: "machines",
	}

	OpenshiftClusterVersionsGVR = schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "clusterversions",
	}
)

func (s *ServiceImpl) GetOpenshiftClusterID(ctx context.Context) (string, error) {
	clusterVersion, err := s.dyno.Resource(OpenshiftClusterVersionsGVR).Get(ctx, "version", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("getting openshift cluster version: %w", err)
	}

	clusterID, ok, err := unstructured.NestedString(clusterVersion.Object, "spec", "clusterID")
	if err != nil {
		return "", fmt.Errorf("getting openshift cluster id: %w", err)
	}

	if !ok {
		return "", fmt.Errorf("openshift cluster id not found")
	}

	return clusterID, nil
}

func (s *ServiceImpl) GetOpenshiftClusterName(ctx context.Context) (string, error) {
	masterSelector := labels.SelectorFromSet(labels.Set{OpenshiftMachineRoleLabel: OpenshiftMasterMachineRole}).String()
	machines, err := s.dyno.Resource(OpenshiftMachinesGVR).
		Namespace(OpenshiftMachineAPINamespace).
		List(ctx, metav1.ListOptions{LabelSelector: masterSelector})
	if err != nil {
		return "", fmt.Errorf("listing openshift machines: %w", err)
	}

	if len(machines.Items) == 0 {
		return "", fmt.Errorf("no openshift master machines found")
	}

	machineLabels := machines.Items[0].GetLabels()

	if machineLabels == nil {
		return "", fmt.Errorf("openshift master machine has no labels")
	}

	clusterName, ok := machineLabels[OpenshiftClusterNameLabel]
	if !ok {
		return "", fmt.Errorf("openshift cluster name label not found")
	}

	return clusterName, nil
}
