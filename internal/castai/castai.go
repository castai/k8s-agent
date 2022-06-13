//go:generate mockgen -destination ./mock/client.go . Client
package castai

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"

	"castai-agent/internal/config"
)

const (
	defaultRetryCount     = 3
	defaultTimeout        = 10 * time.Second
	sendDeltaReadTimeout  = 1 * time.Minute
	totalSendDeltaTimeout = 3 * time.Minute
	headerAPIKey          = "X-API-Key"
)

var (
	hdrContentType     = http.CanonicalHeaderKey("Content-Type")
	hdrContentEncoding = http.CanonicalHeaderKey("Content-Encoding")
	hdrAPIKey          = http.CanonicalHeaderKey(headerAPIKey)
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
	SendLogEvent(ctx context.Context, clusterID string, req *IngestAgentLogsRequest) *IngestAgentLogsResponse
}

// NewClient creates and configures the CAST AI client.
func NewClient(log logrus.FieldLogger, localLog logrus.FieldLogger, rest *resty.Client, deltaHTTPClient *http.Client) Client {
	return &client{
		log:             log,
		localLog:        localLog,
		rest:            rest,
		deltaHTTPClient: deltaHTTPClient,
	}
}

// NewDefaultRestyClient configures a default instance of the resty.Client used to do HTTP requests.
func NewDefaultRestyClient() *resty.Client {
	cfg := config.Get().API

	restyClient := resty.NewWithClient(&http.Client{
		Timeout:   defaultTimeout,
		Transport: createHTTPTransport(),
	})

	restyClient.SetHostURL(cfg.URL)
	restyClient.SetRetryCount(defaultRetryCount)
	restyClient.Header.Set(hdrAPIKey, cfg.Key)

	return restyClient
}

// NewDefaultDeltaHTTPClient configures a default http client used for sending delta requests. Delta requests use a
// different client due to the need to access various low-level features of the http.Client.
func NewDefaultDeltaHTTPClient() *http.Client {
	return &http.Client{
		Timeout:   sendDeltaReadTimeout,
		Transport: createHTTPTransport(),
	}
}

func createHTTPTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

type client struct {
	log             logrus.FieldLogger
	rest            *resty.Client
	deltaHTTPClient *http.Client
	localLog        logrus.FieldLogger
}

func (c *client) SendDelta(ctx context.Context, clusterID string, delta *Delta) error {
	c.log.Debugf("sending delta with items[%d]", len(delta.Items))

	cfg := config.Get().API

	uri, err := url.Parse(fmt.Sprintf("%s/v1/kubernetes/clusters/%s/agent-deltas", cfg.URL, clusterID))
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}

	pipeReader, pipeWriter := io.Pipe()

	go func() {
		defer func() {
			if err := pipeWriter.Close(); err != nil {
				c.log.Errorf("closing gzip pipe: %v", err)
			}
		}()

		gzipWriter := gzip.NewWriter(pipeWriter)
		defer func() {
			if err := gzipWriter.Close(); err != nil {
				c.log.Errorf("closing gzip writer: %v", err)
			}
		}()

		if err := json.NewEncoder(gzipWriter).Encode(delta); err != nil {
			c.log.Errorf("compressing json: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, totalSendDeltaTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri.String(), pipeReader)
	if err != nil {
		return fmt.Errorf("creating delta request: %w", err)
	}

	req.Header.Set(hdrContentType, "application/json")
	req.Header.Set(hdrContentEncoding, "gzip")
	req.Header.Set(hdrAPIKey, cfg.Key)

	var resp *http.Response

	backoff := wait.Backoff{
		Duration: 10 * time.Millisecond,
		Factor:   1.5,
		Jitter:   0.2,
		Steps:    3,
	}
	err = wait.ExponentialBackoffWithContext(ctx, backoff, func() (done bool, err error) {
		resp, err = c.deltaHTTPClient.Do(req)
		if err != nil {
			c.log.Warnf("failed sending delta request: %v", err)
			return false, fmt.Errorf("sending delta request: %w", err)
		}
		return true, nil
	})
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.log.Errorf("closing response body: %v", err)
		}
	}()

	if resp.StatusCode > 399 {
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(resp.Body); err != nil {
			c.log.Errorf("failed reading error response body: %v", err)
		}
		return fmt.Errorf("delta request error status_code=%d body=%s", resp.StatusCode, buf.String())
	}

	c.log.Infof("delta with items[%d] sent, response_code=%d", len(delta.Items), resp.StatusCode)

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

func (c *client) SendLogEvent(ctx context.Context, clusterID string, req *IngestAgentLogsRequest) *IngestAgentLogsResponse {
	body := &IngestAgentLogsResponse{}
	resp, err := c.rest.R().
		SetBody(req).
		SetContext(ctx).
		Post(fmt.Sprintf("/v1/kubernetes/clusters/%s/agent-logs", clusterID))
	if err != nil {
		c.localLog.Errorf("failed to send logs: %v", err)
		return nil
	}
	if resp.IsError() {
		c.localLog.Errorf("send log event: request error status_code=%d body=%s", resp.StatusCode(), resp.Body())
		return nil
	}

	return body
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
