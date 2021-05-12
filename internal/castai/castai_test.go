package castai

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	"github.com/jarcoal/httpmock"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
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
