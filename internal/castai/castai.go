//go:generate mockgen -destination ./mock/client.go . Client
package castai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
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
	hdrContentType        = http.CanonicalHeaderKey("Content-Type")
	hdrContentDisposition = http.CanonicalHeaderKey("Content-Disposition")
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
	client.Header.Set(headerAPIKey, cfg.Key)

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
	cfg := config.Get().API

	uri, err := url.Parse(fmt.Sprintf("https://%s/v1/agent/snapshot", cfg.URL))
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}

	r, w := io.Pipe()
	mw := multipart.NewWriter(w)

	go func() {
		defer func() {
			if err := w.Close(); err != nil {
				c.log.Errorf("closing pipe: %v", err)
			}
		}()
		defer func() {
			if err := mw.Close(); err != nil {
				c.log.Errorf("closing multipart writer: %w", err)
			}
		}()
		if err := writeSnapshotPart(mw, snap); err != nil {
			c.log.Errorf("writing snapshot content: %v", err)
		}
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri.String(), r)
	if err != nil {
		return fmt.Errorf("creating snapshot request: %w", err)
	}

	req.Header.Set(hdrContentType, mw.FormDataContentType())
	req.Header.Set(headerAPIKey, cfg.Key)

	resp, err := c.rest.GetClient().Do(req)
	if err != nil {
		return fmt.Errorf("sending snapshot request: %w", err)
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
		return fmt.Errorf("snapshot request error status_code=%d body=%s", resp.StatusCode, buf.String())
	}

	c.log.Infof(
		"snapshot with nodes[%d], pods[%d] sent, response_code=%d",
		len(snap.NodeList.Items),
		len(snap.PodList.Items),
		resp.StatusCode,
	)

	return nil
}

func writeSnapshotPart(mw *multipart.Writer, snap *Snapshot) error {
	header := textproto.MIMEHeader{}
	header.Set(hdrContentDisposition, `form-data; name="payload"; filename="payload.json"`)
	header.Set(hdrContentType, "application/json")

	bw, err := mw.CreatePart(header)
	if err != nil {
		return fmt.Errorf("creating payload part: %w", err)
	}

	if err := json.NewEncoder(bw).Encode(snap); err != nil {
		return fmt.Errorf("marshaling snapshot payload: %w", err)
	}

	return nil
}
