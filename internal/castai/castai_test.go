package castai

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

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

	c := NewClient(logrus.New(), rest, nil)

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

func TestClient_SendDelta(t *testing.T) {
	httpClient := &http.Client{}
	httpmock.ActivateNonDefault(httpClient)
	defer httpmock.Reset()

	c := NewClient(logrus.New(), nil, httpClient)

	data := json.RawMessage(`"data"`)
	delta := &Delta{
		ClusterID:      uuid.New().String(),
		ClusterVersion: "1.19+",
		FullSnapshot:   true,
		Items: []*DeltaItem{
			{
				Event:     EventAdd,
				Kind:      "Pod",
				Data:      &data,
				CreatedAt: time.Now().UTC(),
			},
		},
	}

	require.NoError(t, os.Setenv("API_KEY", "key"))
	require.NoError(t, os.Setenv("API_URL", "example.com"))

	expectedURI := fmt.Sprintf("https://example.com/v1/kubernetes/clusters/%s/agent-deltas", delta.ClusterID)

	httpmock.RegisterResponder(http.MethodPost, expectedURI, func(r *http.Request) (*http.Response, error) {
		defer r.Body.Close()

		require.Equal(t, "key", r.Header.Get(headerAPIKey))
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.Equal(t, "gzip", r.Header.Get("Content-Encoding"))

		zr, err := gzip.NewReader(r.Body)
		require.NoError(t, err)
		defer zr.Close()

		actualDelta := &Delta{}
		require.NoError(t, json.NewDecoder(zr).Decode(actualDelta))
		require.Equal(t, delta, actualDelta)

		return httpmock.NewStringResponse(203, ""), nil
	})

	err := c.SendDelta(context.Background(), delta.ClusterID, delta)

	require.NoError(t, err)
}

func TestClient_RegisterClusterHTTPError(t *testing.T) {
	rest := resty.New()
	c := NewClient(logrus.New(), rest, nil)

	httpmock.ActivateNonDefault(rest.GetClient())
	defer httpmock.Reset()

	registerClusterReq := &RegisterClusterRequest{Name: "name"}
	registerClusterResp := &RegisterClusterResponse{Cluster{ID: uuid.New().String()}}

	httpmock.RegisterResponder(http.MethodPost, "/v1/kubernetes/external-clusters", func(req *http.Request) (*http.Response, error) {
		resp, err := httpmock.NewJsonResponse(http.StatusRequestTimeout, registerClusterResp)
		require.NoError(t, err)

		// build a test request
		resp.Request = req
		resp.Request.URL.Scheme = "http"
		resp.Request.URL.Host = "/v1/kubernetes/external-clusters"

		return resp, nil
	})

	_, err := c.RegisterCluster(context.Background(), registerClusterReq)

	require.Error(t, err)
	if e, ok := err.(*HTTPError); ok {
		require.Equal(t, "request error status_code=408", e.Message)
		require.Contains(t, e.Request, "request=POST /v1/kubernetes/external-clusters")
		require.Contains(t, e.Request, "TLSHandshake")
		require.Contains(t, e.Response, fmt.Sprintf("{\"id\":\"%s\",\"organizationId\":\"\"}", registerClusterResp.Cluster.ID))
	}
}
