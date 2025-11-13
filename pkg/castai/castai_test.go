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

	"castai-agent/internal/config"
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
	t.Cleanup(config.Reset)
	t.Cleanup(os.Clearenv)

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

func TestCreateTLSConfig(t *testing.T) {
	t.Run("should populate tls.Config RootCAs when valid certificate presented", func(t *testing.T) {
		r := require.New(t)

		t.Cleanup(config.Reset)
		t.Cleanup(os.Clearenv)

		r.NoError(os.Setenv("TLS_CA_CERT_FILE", string(`
-----BEGIN CERTIFICATE-----
MIIDATCCAemgAwIBAgIUPUS4krHP49SF+yYMLHe4nCllKmEwDQYJKoZIhvcNAQEL
BQAwDzENMAsGA1UECgwEVGVzdDAgFw0yMzA5MTMwODM5MzhaGA8yMjE1MDUxMDA4
MzkzOFowDzENMAsGA1UECgwEVGVzdDCCASIwDQYJKoZIhvcNAQEBBQADggEPADCC
AQoCggEBAOVZbDa4/tf3N3VP4Ezvt18d++xrQ+bzjhuE7MWX36NWZ4wUzgmqQXd0
OQWoxYqRGKyI847v29j2BWG17ZmbqarwZHjR98rn9gNtRJgeURlEyAh1pAprhFwb
IBS9vyyCNJtfFFF+lvWvJcU+VKIqWH/9413xDx+OE8tRWNRkS/1CVJg1Nnm3H/IF
lhWAKOYbeKY9q8RtIhb4xNqIc8nmUjDFIjRTarIuf+jDwfFQAPK5pNci+o9KCDgd
Y4lvnGfvPp9XAHnWzTRWNGJQyefZb/SdJjXlic10njfttzKBXi0x8IuV2x98AEPE
2jLXIvC+UBpvMhscdzPfahp5xkYJWx0CAwEAAaNTMFEwHQYDVR0OBBYEFFE48b+V
4E5PWqjpLcUnqWvDDgsuMB8GA1UdIwQYMBaAFFE48b+V4E5PWqjpLcUnqWvDDgsu
MA8GA1UdEwEB/wQFMAMBAf8wDQYJKoZIhvcNAQELBQADggEBAIe82ddHX61WHmyp
zeSiF25aXBqeOUA0ScArTL0fBGi9xZ/8gVU79BvJMyfkaeBKvV06ka6g9OnleWYB
zhBmHBvCL6PsgwLxgzt/dj5ES0K3Ml+7jGmhCKKryzYj/ZvhSMyLlxZqP/nRccBG
y6G3KK4bjzqY4TcEPNs8H4Akc+0SGcPl+AAe65mXPIQhtMkANFLoRuWxMf5JmJke
dYT1GoOjRJpEWCATM+KCXa3UEpRBcXNLeOHZivuqf7n0e1CUD6+0oK4TLxVsTqti
q276VYI/vYmMLRI/iE7Qjn9uGEeR1LWpVngE9jSzSdzByvzw3DwO4sL5B+rv7O1T
9Qgi/No=
-----END CERTIFICATE-----
		`)))
		r.NoError(os.Setenv("API_KEY", "key"))
		r.NoError(os.Setenv("API_URL", "example.com"))

		got, err := createTLSConfig()
		r.NoError(err)
		r.NotNil(got)
		r.NotEmpty(got.RootCAs)
	})

	t.Run("should return error and nil for tls.Config when invalid certificate is given", func(t *testing.T) {
		r := require.New(t)

		t.Cleanup(config.Reset)
		t.Cleanup(os.Clearenv)

		r.NoError(os.Setenv("TLS_CA_CERT_FILE", "certificate"))
		r.NoError(os.Setenv("API_KEY", "key"))
		r.NoError(os.Setenv("API_URL", "example.com"))

		got, err := createTLSConfig()
		r.Error(err)
		r.Nil(got)
	})

	t.Run("should return nil if no certificate is set", func(t *testing.T) {
		r := require.New(t)

		t.Cleanup(config.Reset)
		t.Cleanup(os.Clearenv)

		r.NoError(os.Setenv("API_KEY", "key"))
		r.NoError(os.Setenv("API_URL", "example.com"))

		got, err := createTLSConfig()
		r.NoError(err)
		r.Nil(got)
	})
}
