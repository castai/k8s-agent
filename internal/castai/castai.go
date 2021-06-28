//go:generate mockgen -destination ./mock/client.go . Client
package castai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"

	"castai-agent/internal/config"
)

const (
	defaultRetryCount = 3
	defaultTimeout    = 10 * time.Second
	headerAPIKey      = "X-API-Key"
)

var (
	hdrContentType = http.CanonicalHeaderKey("Content-Type")
	hdrAPIKey      = http.CanonicalHeaderKey(headerAPIKey)
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
	SendDelta(ctx context.Context, delta *Delta) error
	// SendLogEvent sends agent's log event to CAST AI.
	SendLogEvent(ctx context.Context, clusterID string, req *SendLogEventRequest) (*SendLogEventResponse, error)
}

// NewClient creates and configures the CAST AI client.
func NewClient(log logrus.FieldLogger, rest *resty.Client) Client {
	return &client{
		log:  log.WithField("client", "castai"),
		rest: rest,
	}
}

// NewDefaultClient configures a default instance of the resty.Client used to do HTTP requests.
func NewDefaultClient() *resty.Client {
	cfg := config.Get().API

	client := resty.New()
	client.SetHostURL(fmt.Sprintf("https://%s", cfg.URL))
	client.SetRetryCount(defaultRetryCount)
	client.SetTimeout(defaultTimeout)
	client.Header.Set(hdrAPIKey, cfg.Key)

	return client
}

type client struct {
	log  logrus.FieldLogger
	rest *resty.Client
}

func (c *client) SendDelta(ctx context.Context, delta *Delta) error {
	c.log.Debugf("sending delta with items[%d]", len(delta.Items))

	cfg := config.Get().API

	uri, err := url.Parse(fmt.Sprintf("https://%s/v1/agent/cluster-delta", cfg.URL))
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}

	r, w := io.Pipe()

	go func() {
		defer func() {
			if err := w.Close(); err != nil {
				c.log.Errorf("closing pipe: %v", err)
			}
		}()

		if err := json.NewEncoder(w).Encode(delta); err != nil {
			c.log.Errorf("marshaling delta: %v", err)
		}
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri.String(), r)
	if err != nil {
		return fmt.Errorf("creating delta request: %w", err)
	}

	req.Header.Set(hdrContentType, "application/json")
	req.Header.Set(hdrAPIKey, cfg.Key)

	resp, err := c.rest.GetClient().Do(req)
	if err != nil {
		return fmt.Errorf("sending delta request: %w", err)
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

func (c *client) SendLogEvent(
	ctx context.Context,
	clusterID string,
	req *SendLogEventRequest,
) (*SendLogEventResponse, error) {
	body := &SendLogEventResponse{}
	resp, err := c.rest.R().
		SetBody(req).
		SetContext(ctx).
		Post(fmt.Sprintf("/v1/agent/logs/%s", clusterID))
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
	resp, err := r.Post(fmt.Sprintf("/v1/agent/config/%s", clusterID))
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("request error status_code=%d body=%s", resp.StatusCode(), resp.Body())
	}

	return body, nil
}
