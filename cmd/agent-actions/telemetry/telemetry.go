package telemetry

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
)

const (
	headerAPIKey = "X-API-Key"
)

var (
	hdrAPIKey = http.CanonicalHeaderKey(headerAPIKey)
)

type Client interface {
	GetActions(ctx context.Context, clusterID string) ([]*AgentAction, error)
	AckActions(ctx context.Context, clusterID string, ack []*AgentActionAck) error
}

func NewClient(log *logrus.Logger, rest *resty.Client) Client {
	return &client{
		log:  log,
		rest: rest,
	}
}

// NewDefaultClient configures a default instance of the resty.Client used to do HTTP requests.
func NewDefaultClient(url, key string, level logrus.Level) *resty.Client {
	client := resty.New()
	client.SetHostURL(fmt.Sprintf("https://%s", url))
	client.Header.Set(hdrAPIKey, key)
	if level == logrus.TraceLevel {
		client.SetDebug(true)
	}

	return client
}

type client struct {
	log  *logrus.Logger
	rest *resty.Client
}

func (c *client) GetActions(ctx context.Context, clusterID string) ([]*AgentAction, error) {
	res := &GetAgentActionsResponse{}
	resp, err := c.rest.R().
		SetContext(ctx).
		SetResult(res).
		Get(fmt.Sprintf("/api/clusters/%s/agent-actions", clusterID))
	if err != nil {
		return nil, fmt.Errorf("failed to request agent-actions: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("get agent-actions: request error host=%s, status_code=%d body=%s", c.rest.HostURL, resp.StatusCode(), resp.Body())
	}
	return res.Actions, nil
}

func (c *client) AckActions(ctx context.Context, clusterID string, ack []*AgentActionAck) error {
	req := &AckAgentActionsRequest{Ack: ack}
	resp, err := c.rest.R().
		SetContext(ctx).
		SetBody(req).
		Delete(fmt.Sprintf("/api/clusters/%s/agent-actions", clusterID))
	if err != nil {
		return fmt.Errorf("failed to request agent-actions ack: %v", err)
	}
	if resp.IsError() {
		return fmt.Errorf("ack agent-actions: request error status_code=%d body=%s", resp.StatusCode(), resp.Body())
	}
	return nil
}
