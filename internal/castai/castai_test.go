package castai

import (
	"castai-agent/internal/config"
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

func TestCreateTLSConfig(t *testing.T) {
	t.Run("should populate tls.Config RootCAs when valid certificate presented", func(t *testing.T) {
		r := require.New(t)

		t.Cleanup(config.Reset)
		t.Cleanup(os.Clearenv)

		r.NoError(os.Setenv("TLS_CA_CERT_FILE", string(`
-----BEGIN CERTIFICATE-----
MIIC/zCCAeegAwIBAgIUHD6uJtqPjkKWI0OyZ3ZSyXJt5LUwDQYJKoZIhvcNAQEL
BQAwDzENMAsGA1UECgwEVGVzdDAeFw0yMzA5MDgwODMyMTNaFw0yNDA5MDcwODMy
MTNaMA8xDTALBgNVBAoMBFRlc3QwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEK
AoIBAQD5PhKa0D8VfVai7cZhDpBdFLHcEaYlYtgm3yyDMe1jxWn7SYl5GbC7+8+o
C2hZwYVXqik65BAg6QSf0waBy98c4uuD9yw04mLxRtdnsAxdXIewO/i0+zUqLayR
HXTTU9LEAENu8Kk27ms8nzlx9jzM/2cNgjmXmksyZrDkdeGcs2uwWV8ZwcAcW+aV
CNHow/WG+3H5pD4rI0DLahUiph+CHDT7YZchyjhNVq756UfaJ57sTRmS13uwOTQ4
ZdCxKgO656yManYkm2k9XaIT+vqIy0m8OxkWMulsTy0WfwmBCCNMUICdr95ppycl
+dkHyqyvty8VeB0kteVh+j3TX4MXAgMBAAGjUzBRMB0GA1UdDgQWBBQVK59ct3yY
2/sjwFGwoP9+eV9MyzAfBgNVHSMEGDAWgBQVK59ct3yY2/sjwFGwoP9+eV9MyzAP
BgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQCjCuV2urgJtZWvX1df
5FGh4o3jAC8dEQZMJvW9C1/gPlDGs/z9/2Fo74hYDW87hJnayPFaveRDYhHul6sN
J1sUKDdDUkVIarRS5/G39xNCvjYFRuag2vqO6PzxEe0h8NtVrCxkvG1qrgV2y0Tx
0mZiX8xgjAocjdZV4kPaVL+b5+N8LiR/yw1WQLPNvuZF4hsLqB0uaDT5ekFRnUM1
yxZ0LjlYV+vtoEgA9tLwVGR5Tu30ggVMuZ4ZwIu6XMR3QCNHz7HjKFVmG3t8RgjI
OiGvAsXa3skJBqeNXUXQR/GsRm7UfJV9gVt2sJvmeHPx1PfQytWW8KFJJL2Qe+7p
rWFE
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
