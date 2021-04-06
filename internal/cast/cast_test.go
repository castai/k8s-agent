package cast

import (
	"castai-agent/internal/services/collector"
	"context"
	"encoding/json"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	"github.com/jarcoal/httpmock"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"testing"
)

func TestClient_RegisterCluster(t *testing.T) {
	rest := resty.New()
	httpmock.ActivateNonDefault(rest.GetClient())
	defer httpmock.Reset()

	c := NewClient(logrus.New(), rest)

	registerClusterReq := &RegisterClusterRequest{Name: "name"}
	registerClusterResp := &RegisterClusterResponse{Cluster{ID: uuid.New().String()}}

	httpmock.RegisterResponder(http.MethodPost, "/v1/kubernetes/external-clusters", func(req *http.Request) (*http.Response, error) {
		actualRequest := &RegisterClusterRequest{}
		require.NoError(t, json.NewDecoder(req.Body).Decode(actualRequest))
		require.Equal(t, registerClusterReq, actualRequest)
		return httpmock.NewJsonResponse(http.StatusOK, registerClusterResp)
	})

	got, err := c.RegisterCluster(context.Background(), registerClusterReq)

	require.NoError(t, err)
	require.Equal(t, registerClusterResp, got)
}

func TestClient_SendClusterSnapshot(t *testing.T) {
	rest := resty.New()
	httpmock.ActivateNonDefault(rest.GetClient())
	defer httpmock.Reset()

	c := NewClient(logrus.New(), rest)

	snapshot := &Snapshot{
		ClusterID: uuid.New().String(),
		ClusterData: &collector.ClusterData{
			NodeList: &corev1.NodeList{
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test",
						},
					},
				},
			},
			PodList: &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test",
						},
					},
				},
			},
		},
	}

	httpmock.RegisterResponder(http.MethodPost, "/v1/agent/eks-snapshot", func(req *http.Request) (*http.Response, error) {
		actualRequest := &SnapshotRequest{}
		require.NoError(t, json.NewDecoder(req.Body).Decode(actualRequest))

		actualSnapshot := &Snapshot{}
		require.NoError(t, json.Unmarshal(actualRequest.Payload, actualSnapshot))

		require.Equal(t, snapshot, actualSnapshot)

		return httpmock.NewStringResponse(http.StatusNoContent, "ok"), nil
	})

	err := c.SendClusterSnapshot(context.Background(), snapshot)

	require.NoError(t, err)
}