//go:generate mockgen -destination ./mock/client.go . Client
package castai

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/http2"
	"k8s.io/apimachinery/pkg/util/wait"

	"castai-agent/internal/config"
)

const (
	defaultRetryCount     = 3
	headerAPIKey          = "X-Api-Key"
	headerContinuityToken = "Continuity-Token"
	headerContentType     = "Content-Type"
	headerContentEncoding = "Content-Encoding"
	headerUserAgent       = "User-Agent"

	respHeaderRequestID = "X-Castai-Request-Id"
)

var (
	ErrInvalidContinuityToken = errors.New("invalid continuity token")
)

// Client responsible for communication between the agent and CAST AI API.
type Client interface {
	// RegisterCluster sends a request to CAST AI containing discovered cluster properties used to authenticate the
	// cluster and register it.
	RegisterCluster(ctx context.Context, req *RegisterClusterRequest) (*RegisterClusterResponse, error)
	// ExchangeAgentTelemetry is used to send agent information (e.g. version)
	// as well as poll CAST AI for agent configuration which can be updated via UI or other means.
	ExchangeAgentTelemetry(ctx context.Context, clusterID string, req *AgentTelemetryRequest) (*AgentTelemetryResponse, error)
	// SendDelta sends the kubernetes state change to CAST AI. Function is noop when items are empty.
	SendDelta(ctx context.Context, clusterID string, delta *Delta) error
	// SendLogEvent sends agent's log event to CAST AI.
	SendLogEvent(ctx context.Context, clusterID string, req *IngestAgentLogsRequest) (*IngestAgentLogsResponse, error)
}

// NewClient creates and configures the CAST AI client.
func NewClient(log logrus.FieldLogger, rest *resty.Client, deltaHTTPClient *http.Client) Client {
	return &client{
		log:             log,
		rest:            rest,
		deltaHTTPClient: deltaHTTPClient,
	}
}

// NewDefaultRestyClient configures a default instance of the resty.Client used to do HTTP requests.
func NewDefaultRestyClient() (*resty.Client, error) {
	cfg := config.Get().API

	clientTransport, err := createHTTPTransport()
	if err != nil {
		return nil, err
	}

	restyClient := resty.NewWithClient(&http.Client{
		Timeout:   cfg.Timeout,
		Transport: clientTransport,
	})

	restyClient.SetBaseURL(cfg.URL)
	restyClient.SetRetryCount(defaultRetryCount)
	restyClient.Header.Set(headerAPIKey, cfg.Key)
	restyClient.Header.Set(headerUserAgent, fmt.Sprintf("castai-agent/%s", config.VersionInfo.Version))
	if host := cfg.HostHeaderOverride; host != "" {
		restyClient.Header.Set("Host", host)
	}
	addUA(restyClient.Header)

	return restyClient, nil
}

// NewDefaultDeltaHTTPClient configures a default http client used for sending delta requests. Delta requests use a
// different client due to the need to access various low-level features of the http.Client.
func NewDefaultDeltaHTTPClient() (*http.Client, error) {
	clientTransport, err := createHTTPTransport()
	if err != nil {
		return nil, err
	}

	return &http.Client{
		Timeout:   config.Get().API.DeltaReadTimeout,
		Transport: clientTransport,
	}, nil
}

func createHTTPTransport() (*http.Transport, error) {
	tlsConfig, err := createTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("creating TLS config: %v", err)
	}

	t1 := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       tlsConfig,
	}

	t2, err := http2.ConfigureTransports(t1)
	if err != nil {
		return nil, fmt.Errorf("failed to configure HTTP2 transport: %w", err)
	} else {
		// Adding timeout settings to the http2 transport to prevent bad tcp connection hanging the requests for too long
		// Doc: https://pkg.go.dev/golang.org/x/net/http2#Transport
		//  - ReadIdleTimeout is the time before a ping is sent when no frame has been received from a connection
		//  - PingTimeout is the time before the TCP connection being closed if a Ping response is not received
		// So in total, if a TCP connection goes bad, it would take the combined time before the TCP connection is closed
		t2.ReadIdleTimeout = 10 * time.Second
		t2.PingTimeout = 5 * time.Second
	}

	return t1, nil
}

func createTLSConfig() (*tls.Config, error) {
	cert := config.Get().TLS
	if cert == nil {
		return nil, nil
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM([]byte(cert.CACertFile)) {
		return nil, fmt.Errorf("failed to add root certificate to CA pool")
	}

	return &tls.Config{
		RootCAs: certPool,
	}, nil
}

type client struct {
	log             logrus.FieldLogger
	rest            *resty.Client
	deltaHTTPClient *http.Client
	continuityToken string
}

func (c *client) SendDelta(ctx context.Context, clusterID string, delta *Delta) error {
	roundtripTimer := NewTimer()

	log := c.log.WithFields(logrus.Fields{
		"full_snapshot": delta.FullSnapshot,
		"items":         len(delta.Items),
	})

	log.Debugf("sending delta")

	cfg := config.Get().API

	uri, err := url.Parse(fmt.Sprintf("%s/v1/kubernetes/clusters/%s/agent-deltas", cfg.URL, clusterID))
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}

	pipeReader, pipeWriter := io.Pipe()

	go func() {
		defer func() {
			if err := pipeWriter.Close(); err != nil {
				log.Errorf("closing gzip pipe: %v", err)
			}
		}()

		gzipWriter := gzip.NewWriter(pipeWriter)
		defer func() {
			if err := gzipWriter.Close(); err != nil {
				log.Errorf("closing gzip writer: %v", err)
			}
		}()

		if err := json.NewEncoder(gzipWriter).Encode(delta); err != nil {
			log.Errorf("compressing json: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, cfg.TotalSendDeltaTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri.String(), pipeReader)
	if err != nil {
		return fmt.Errorf("creating delta request: %w", err)
	}

	req.Header.Set(headerContentType, "application/json")
	req.Header.Set(headerContentEncoding, "gzip")
	req.Header.Set(headerAPIKey, cfg.Key)
	req.Header.Set(headerContinuityToken, c.continuityToken)
	addUA(req.Header)

	if host := cfg.HostHeaderOverride; host != "" {
		req.Header.Set("Host", host)
	}

	var resp *http.Response

	// Retry 3 times with 25ms, 250ms and 2.5s sleep intervals (not accounting for jitter).
	backoff := wait.Backoff{
		Duration: 25 * time.Millisecond,
		Factor:   10,
		Jitter:   0.2,
		Steps:    3,
	}
	numAttempts := 0
	err = wait.ExponentialBackoffWithContext(ctx, backoff, func(context.Context) (done bool, err error) {
		numAttempts++
		log = log.WithField("attempts", numAttempts)

		resp, err = c.deltaHTTPClient.Do(req)
		if err != nil {
			log.Warnf("failed sending delta request: %v", err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Errorf("closing response body: %v", err)
		}
	}()

	roundtripTimer.Stop()

	log = log.WithFields(logrus.Fields{
		"roundtrip_duration_ms": roundtripTimer.Duration().Milliseconds(),
		"response_code":         resp.StatusCode,
	})

	if resp.StatusCode > 399 {
		requestID := resp.Header.Get(respHeaderRequestID)
		responseStatusCode := resp.StatusCode
		responseBody, err := readAllString(resp.Body)
		if err != nil {
			log.Errorf("failed reading error response body: %v", err)
		}

		if strings.Contains(responseBody, ErrInvalidContinuityToken.Error()) {
			return ErrInvalidContinuityToken
		}

		return DeltaRequestError{
			requestID:  requestID,
			statusCode: responseStatusCode,
			body:       responseBody,
		}
	}
	c.continuityToken = resp.Header.Get(headerContinuityToken)
	log.Infof("delta upload finished")

	return nil
}

func (c *client) RegisterCluster(ctx context.Context, req *RegisterClusterRequest) (*RegisterClusterResponse, error) {
	body := &RegisterClusterResponse{}
	resp, err := c.rest.R().
		SetBody(req).
		SetResult(body).
		SetContext(ctx).
		Post("/v1/kubernetes/external-clusters")
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("request error status_code=%d body=%s", resp.StatusCode(), resp.Body())
	}

	return body, nil
}

func (c *client) SendLogEvent(ctx context.Context, clusterID string, req *IngestAgentLogsRequest) (*IngestAgentLogsResponse, error) {
	body := &IngestAgentLogsResponse{}
	resp, err := c.rest.R().
		SetBody(req).
		SetContext(ctx).
		Post(fmt.Sprintf("/v1/kubernetes/clusters/%s/agent-logs", clusterID))
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("request error status_code=%d body=%s", resp.StatusCode(), resp.Body())
	}

	return body, nil
}

func (c *client) ExchangeAgentTelemetry(ctx context.Context, clusterID string, req *AgentTelemetryRequest) (*AgentTelemetryResponse, error) {
	body := &AgentTelemetryResponse{}
	r := c.rest.R().
		SetResult(body).
		SetContext(ctx)
	if req != nil {
		r.SetBody(req)
	}
	resp, err := r.Post(fmt.Sprintf("/v1/kubernetes/clusters/%s/agent-config", clusterID))
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("request error status_code=%d body=%s", resp.StatusCode(), resp.Body())
	}

	return body, nil
}

func addUA(header http.Header) {
	version := "unknown"
	if vi := config.VersionInfo; vi != nil {
		version = vi.Version
	}
	header.Set(headerUserAgent, fmt.Sprintf("castai-agent/%s", version))
}

func readAllString(r io.Reader) (string, error) {
	var buffer bytes.Buffer
	_, err := buffer.ReadFrom(r)
	if err != nil {
		return "", err
	}
	return buffer.String(), nil
}
