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

type Client interface {
	RegisterCluster(ctx context.Context, req *RegisterClusterRequest) (*RegisterClusterResponse, error)
	SendClusterSnapshot(snap *Snapshot) error
}

func NewClient(log logrus.FieldLogger, rest *resty.Client) Client {
	return &client{
		log:  log.WithField("client", "cast"),
		rest: rest,
	}
}

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

func (c *client) SendClusterSnapshot(snap *Snapshot) error {
	payload, err := json.Marshal(snap)
	if err != nil {
		return err
	}

	resp, err := c.rest.R().
		SetBody(&Request{Payload: payload}).
		SetResult(&RegisterClusterResponse{}).
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
