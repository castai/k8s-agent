package openshift

import (
	"context"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakedynamic "k8s.io/client-go/dynamic/fake"

	"castai-agent/internal/castai"
	mock_castai "castai-agent/internal/castai/mock"
	"castai-agent/internal/config"
	"castai-agent/internal/services/controller/scheme"
	"castai-agent/internal/services/discovery"
	mock_discovery "castai-agent/internal/services/discovery/mock"
	"castai-agent/internal/services/providers/types"
	"castai-agent/pkg/cloud"
)

func TestProvider_RegisterCluster(t *testing.T) {
	regReq := &castai.RegisterClusterRequest{
		ID:   uuid.New(),
		Name: "test-cluster",
		Openshift: &castai.OpenshiftParams{
			CSP:         string(cloud.AWS),
			Region:      "us-east-1",
			ClusterName: "test-cluster",
			InternalID:  uuid.New().String(),
		},
	}

	tests := []struct {
		name                 string
		config               func(t *testing.T)
		discoveryServiceMock func(t *testing.T, mock *mock_discovery.MockService)
	}{
		{
			name: "should autodiscover cluster properties from kubernetes state using the discovery service",
			config: func(t *testing.T) {
				r := require.New(t)
				r.NoError(os.Setenv("API_KEY", "123"))
				r.NoError(os.Setenv("API_URL", "test"))
			},
			discoveryServiceMock: func(t *testing.T, discoveryService *mock_discovery.MockService) {
				discoveryService.EXPECT().GetKubeSystemNamespaceID(gomock.Any()).Return(&regReq.ID, nil)
				discoveryService.EXPECT().GetCSPAndRegion(gomock.Any()).Return(cloud.Cloud(regReq.Openshift.CSP), regReq.Openshift.Region, nil)
				discoveryService.EXPECT().GetOpenshiftClusterID(gomock.Any()).Return(regReq.Openshift.InternalID, nil)
				discoveryService.EXPECT().GetOpenshiftClusterName(gomock.Any()).Return(regReq.Name, nil)
			},
		},
		{
			name: "should use config values if provided",
			config: func(t *testing.T) {
				r := require.New(t)
				r.NoError(os.Setenv("API_KEY", "123"))
				r.NoError(os.Setenv("API_URL", "test"))
				r.NoError(os.Setenv("OPENSHIFT_CSP", regReq.Openshift.CSP))
				r.NoError(os.Setenv("OPENSHIFT_REGION", regReq.Openshift.Region))
				r.NoError(os.Setenv("OPENSHIFT_CLUSTER_NAME", regReq.Name))
				r.NoError(os.Setenv("OPENSHIFT_INTERNAL_ID", regReq.Openshift.InternalID))
			},
			discoveryServiceMock: func(t *testing.T, discoveryService *mock_discovery.MockService) {
				discoveryService.EXPECT().GetKubeSystemNamespaceID(gomock.Any()).Return(&regReq.ID, nil)
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			r := require.New(t)

			if test.config != nil {
				test.config(t)
				t.Cleanup(config.Reset)
				t.Cleanup(os.Clearenv)
			}

			mockctrl := gomock.NewController(t)
			castaiclient := mock_castai.NewMockClient(mockctrl)
			discoveryService := mock_discovery.NewMockService(mockctrl)

			p := New(discoveryService, nil)

			if test.discoveryServiceMock != nil {
				test.discoveryServiceMock(t, discoveryService)
			}

			expected := &types.ClusterRegistration{
				ClusterID:      regReq.ID.String(),
				OrganizationID: uuid.New().String(),
			}

			castaiclient.EXPECT().RegisterCluster(gomock.Any(), regReq).
				Return(&castai.RegisterClusterResponse{
					Cluster: castai.Cluster{
						ID:             regReq.ID.String(),
						OrganizationID: expected.OrganizationID,
					},
				}, nil)

			got, err := p.RegisterCluster(context.Background(), castaiclient)

			r.NoError(err)
			r.Equal(expected, got)
		})
	}
}

func TestOpenshift_FilterSpot(t *testing.T) {
	r := require.New(t)

	onDemand, err := discovery.UnstructuredMachine(`
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
spec:
  providerSpec:
    value: {}
status:
  providerStatus:
    instanceId: i-0b9c9c9c9c9c9c9c9
`)
	r.NoError(err)

	spot, err := discovery.UnstructuredMachine(`
apiVersion: machine.openshift.io/v1beta1
kind: Machine
metadata:
  labels:
    machine.openshift.io/cluster-api-cluster: foo-bar
    machine.openshift.io/cluster-api-machine-role: worker
    machine.openshift.io/cluster-api-machine-type: worker
    machine.openshift.io/instance-type: m5.2xlarge
    machine.openshift.io/region: eu-central-1
    machine.openshift.io/zone: eu-central-1a
  name: foo-bar-worker-0
  namespace: openshift-machine-api
  uid: 803fbff1-bab4-412c-912b-0ba7f0ef1dcf
spec:
  providerSpec:
    value:
      spotMarketOptions: {}
status:
  providerStatus:
    instanceId: i-0b9c9c9c9c9c9c9cf
`)
	r.NoError(err)

	nonExisting, err := discovery.UnstructuredMachine(`
apiVersion: machine.openshift.io/v1beta1
kind: Machine
metadata:
  labels:
    machine.openshift.io/cluster-api-cluster: foo-bar
    machine.openshift.io/cluster-api-machine-role: worker
    machine.openshift.io/cluster-api-machine-type: worker
    machine.openshift.io/instance-type: m5.2xlarge
    machine.openshift.io/region: eu-central-1
    machine.openshift.io/zone: eu-central-1a
  name: foo-bar-worker-1
  namespace: openshift-machine-api
  uid: 803fbff1-bab4-412c-912b-0ba7f0ef1dcc
spec:
  providerSpec:
    value:
status:
  providerStatus:
    instanceId: i-0b9c9c9c9c9c9c9cc
`)
	r.NoError(err)

	objects := []runtime.Object{
		onDemand,
		spot,
		nonExisting,
	}

	nodes := []*v1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "spot"},
			Spec: v1.NodeSpec{
				ProviderID: "aws:///eu-central-1/i-0b9c9c9c9c9c9c9cf",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "on-demand"},
			Spec: v1.NodeSpec{
				ProviderID: "aws:///eu-central-1/i-0b9c9c9c9c9c9c9c9",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "non-existing"},
			Spec: v1.NodeSpec{
				ProviderID: "aws:///eu-central-1/i-99999999999999999",
			},
		},
	}

	dyno := fakedynamic.NewSimpleDynamicClient(scheme.Scheme, objects...)

	p := New(nil, dyno)

	spots, err := p.FilterSpot(context.Background(), nodes)

	r.NoError(err)
	r.Equal([]*v1.Node{nodes[0]}, spots)
}
