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

func TestCastwareInstallMethod_Validate(t *testing.T) {
	tests := []struct {
		name        string
		method      *CastwareInstallMethod
		expectError bool
	}{
		{
			name:        "nil method is valid",
			method:      nil,
			expectError: false,
		},
		{
			name: "unspecified method is valid",
			method: func() *CastwareInstallMethod {
				m := CastwareInstallMethodUnspecified
				return &m
			}(),
			expectError: false,
		},
		{
			name: "operator method is valid",
			method: func() *CastwareInstallMethod {
				m := CastwareInstallMethodOperator
				return &m
			}(),
			expectError: false,
		},
		{
			name: "invalid method returns error",
			method: func() *CastwareInstallMethod {
				m := CastwareInstallMethod(999)
				return &m
			}(),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.method.validate()
			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), "unsupported castware_install_method")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_RegisterCluster_WithInstallMethod(t *testing.T) {
	rest := resty.New()
	httpmock.ActivateNonDefault(rest.GetClient())
	defer httpmock.Reset()

	c := NewClient(logrus.New(), rest, nil)

	operatorMethod := CastwareInstallMethodOperator
	registerClusterReq := &RegisterClusterRequest{
		Name:                  "test-cluster",
		CastwareInstallMethod: &operatorMethod,
	}
	registerClusterResp := &RegisterClusterResponse{Cluster{ID: uuid.New().String()}}

	httpmock.RegisterResponder(http.MethodPost, "/v1/kubernetes/external-clusters", func(req *http.Request) (*http.Response, error) {
		actualRequest := &RegisterClusterRequest{}
		require.NoError(t, json.NewDecoder(req.Body).Decode(actualRequest))
		require.NotNil(t, actualRequest.CastwareInstallMethod)
		require.Equal(t, CastwareInstallMethodOperator, *actualRequest.CastwareInstallMethod)
		return httpmock.NewJsonResponse(http.StatusOK, registerClusterResp)
	})

	got, err := c.RegisterCluster(context.Background(), registerClusterReq)

	require.NoError(t, err)
	require.Equal(t, registerClusterResp, got)
}

func TestClient_RegisterCluster_WithNilInstallMethod(t *testing.T) {
	rest := resty.New()
	httpmock.ActivateNonDefault(rest.GetClient())
	defer httpmock.Reset()

	c := NewClient(logrus.New(), rest, nil)

	registerClusterReq := &RegisterClusterRequest{
		Name:                  "test-cluster",
		CastwareInstallMethod: nil,
	}
	registerClusterResp := &RegisterClusterResponse{Cluster{ID: uuid.New().String()}}

	httpmock.RegisterResponder(http.MethodPost, "/v1/kubernetes/external-clusters", func(req *http.Request) (*http.Response, error) {
		actualRequest := &RegisterClusterRequest{}
		require.NoError(t, json.NewDecoder(req.Body).Decode(actualRequest))
		require.Nil(t, actualRequest.CastwareInstallMethod)
		require.Equal(t, "test-cluster", actualRequest.Name)
		return httpmock.NewJsonResponse(http.StatusOK, registerClusterResp)
	})

	got, err := c.RegisterCluster(context.Background(), registerClusterReq)

	require.NoError(t, err)
	require.Equal(t, registerClusterResp, got)
}

func TestClient_RegisterCluster_WithInvalidInstallMethod(t *testing.T) {
	rest := resty.New()
	httpmock.ActivateNonDefault(rest.GetClient())
	defer httpmock.Reset()

	c := NewClient(logrus.New(), rest, nil)

	invalidMethod := CastwareInstallMethod(999)
	registerClusterReq := &RegisterClusterRequest{
		Name:                  "test-cluster",
		CastwareInstallMethod: &invalidMethod,
	}

	_, err := c.RegisterCluster(context.Background(), registerClusterReq)

	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported castware_install_method value: 999")
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
