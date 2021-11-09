//go:generate mockgen -source castai.go -destination ./mock/client.go . Client
package castai

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
)

const (
	defaultRetryCount = 3
	defaultTimeout    = 10 * time.Second
	headerAPIKey      = "X-API-Key"
)

var (
	hdrAPIKey = http.CanonicalHeaderKey(headerAPIKey)
)

var DoNotSendLogs = struct{}{}

func DoNotSendLogsCtx() context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, DoNotSendLogs, "true")
	return ctx
}

// Client responsible for communication between the agent and CAST AI API.
type Client interface {
	SendLogEvent(ctx context.Context, clusterID string, req *IngestAgentLogsRequest) *IngestAgentLogsResponse
}

// NewClient creates and configures the CAST AI client.
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
	client.SetRetryCount(defaultRetryCount)
	client.SetTimeout(defaultTimeout)
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

func (c *client) SendLogEvent(ctx context.Context, clusterID string, req *IngestAgentLogsRequest) *IngestAgentLogsResponse {
	body := &IngestAgentLogsResponse{}
	resp, err := c.rest.R().
		SetBody(req).
		SetContext(ctx).
		Post(fmt.Sprintf("/v1/kubernetes/clusters/%s/agent-logs", clusterID))
	log := c.log.WithContext(DoNotSendLogsCtx())
	if err != nil {
		log.Errorf("failed to send logs: %v", err)
		return nil
	}
	if resp.IsError() {
		log.Errorf("send log event: request error host=%s status_code=%d body=%s", c.rest.HostURL, resp.StatusCode(), resp.Body())
		return nil
	}

	return body
}
