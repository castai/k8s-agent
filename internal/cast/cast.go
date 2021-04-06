//go:generate mockgen -destination ./mock/client.go . Client
package cast

import (
	"castai-agent/internal/config"
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
	"time"
)

const (
	defaultRetryCount = 3
	defaultTimeout    = 10 * time.Second
)

// Client responsible for communication between the agent and CAST AI API.
type Client interface {
	// RegisterCluster sends a request to CAST AI containing discovered cluster properties used to authenticate the
	// cluster and register it.
	RegisterCluster(ctx context.Context, req *RegisterClusterRequest) (*RegisterClusterResponse, error)
	// SendClusterSnapshot sends a cluster snapshot to CAST AI to enable savings estimations / autoscaling / etc.
	SendClusterSnapshot(ctx context.Context, snap *Snapshot) error
}

// NewClient creates and configures the CAST AI client.
func NewClient(log logrus.FieldLogger, rest *resty.Client) Client {
	return &client{
		log:  log.WithField("client", "cast"),
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
	client.Header.Set("X-API-Key", cfg.Key)
	client.Header.Set("Content-Type", "application/json")

	return client
}

type client struct {
	log  logrus.FieldLogger
	rest *resty.Client
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

	c.log.Infof("cluster registered: %+v", body)

	return body, nil
}

func (c *client) SendClusterSnapshot(ctx context.Context, snap *Snapshot) error {
	payload, err := json.Marshal(snap)
	if err != nil {
		return err
	}

	resp, err := c.rest.R().
		SetBody(&SnapshotRequest{Payload: payload}).
		SetResult(&RegisterClusterResponse{}).
		SetContext(ctx).
		Post("/v1/agent/eks-snapshot")
	if err != nil {
		return err
	}
	if resp.IsError() {
		return fmt.Errorf("request error status_code=%d body=%s", resp.StatusCode(), resp.Body())
	}

	c.log.Infof(
		"snapshot with nodes[%d], pods[%d] sent, response_code=%d",
		len(snap.NodeList.Items),
		len(snap.PodList.Items),
		resp.StatusCode(),
	)

	return nil
}
