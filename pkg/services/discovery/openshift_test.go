package discovery

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	fakedynamic "k8s.io/client-go/dynamic/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"

	"castai-agent/internal/services/controller/scheme"
)

func TestServiceImpl_GetOpenshiftClusterID(t *testing.T) {
	r := require.New(t)

	version, err := UnstructuredVersion(`
apiVersion: config.openshift.io/v1
kind: ClusterVersion
metadata:
  name: version
  uid: dc0570f9-2c46-40b1-a5d0-c7233e82c7b6
spec:
  clusterID: 91d87440-5173-47f0-aca5-65bc0144ad30
`)
	r.NoError(err)

	clientset := fakeclientset.NewSimpleClientset(version)
	dyno := fakedynamic.NewSimpleDynamicClient(scheme.Scheme, version)

	s := New(clientset, dyno)

	internalID, err := s.GetOpenshiftClusterID(context.Background())

	r.NoError(err)
	r.Equal("91d87440-5173-47f0-aca5-65bc0144ad30", internalID)
}

func TestServiceImpl_GetOpenshiftClusterName(t *testing.T) {
	r := require.New(t)

	masterMachine, err := UnstructuredMachine(`
apiVersion: machine.openshift.io/v1beta1
kind: Machine
metadata:
  labels:
    machine.openshift.io/cluster-api-cluster: foo-bar
    machine.openshift.io/cluster-api-machine-role: master
    machine.openshift.io/cluster-api-machine-type: master
    machine.openshift.io/instance-type: m5.2xlarge
    machine.openshift.io/region: eu-central-1
    machine.openshift.io/zone: eu-central-1a
  name: foo-bar-master-0
  namespace: openshift-machine-api
  uid: 803fbff1-bab4-412c-912b-0ba7f0ef1dcd
`)
	r.NoError(err)

	clientset := fakeclientset.NewSimpleClientset(masterMachine)
	dyno := fakedynamic.NewSimpleDynamicClient(scheme.Scheme, masterMachine)

	s := New(clientset, dyno)

	clusterName, err := s.GetOpenshiftClusterName(context.Background())

	r.NoError(err)
	r.Equal("foo-bar", clusterName)
}
