package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	metadataURL = "http://169.254.169.254/metadata/instance"
	timeout     = 2 * time.Second
)

type ComputeMetadata struct {
	Location       string `json:"location"`
	ResourceGroup  string `json:"resourceGroupName"`
	SubscriptionID string `json:"subscriptionId"`
}

type instanceMetadata struct {
	Compute *ComputeMetadata `json:"compute,omitempty"`
}

func NewClient(log logrus.FieldLogger) Client {
	return Client{
		log: log,
	}
}

type Client struct {
	log      logrus.FieldLogger
	metadata ComputeMetadata
	syncOnce sync.Once
}

func (c *Client) GetLocation() (string, error) {
	c.syncOnce.Do(c.loadMetadata)
	if c.metadata.Location == "" {
		return "", fmt.Errorf("failed to get location metadata")
	}

	return c.metadata.Location, nil
}

func (c *Client) GetResourceGroup() (string, error) {
	c.syncOnce.Do(c.loadMetadata)
	if c.metadata.ResourceGroup == "" {
		return "", fmt.Errorf("failed to get resource group metadata")
	}

	return c.metadata.ResourceGroup, nil
}

func (c *Client) GetSubscriptionID() (string, error) {
	c.syncOnce.Do(c.loadMetadata)
	if c.metadata.SubscriptionID == "" {
		return "", fmt.Errorf("failed to get subscription metadata")
	}

	return c.metadata.SubscriptionID, nil
}

func (c *Client) loadMetadata() {
	metadata, err := getInstanceMetadata(context.Background(), c.log)
	if err != nil {
		c.log.Errorf("failed to retrieve instance metadata: %v", err)
		return
	}
	c.metadata = *metadata
}

func getInstanceMetadata(ctx context.Context, log logrus.FieldLogger) (*ComputeMetadata, error) {
	req, err := http.NewRequest("GET", metadataURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Metadata", "true")
	q := req.URL.Query()
	q.Add("format", "json")
	q.Add("api-version", "2021-05-01")
	req.URL.RawQuery = q.Encode()

	client := &http.Client{
		Timeout: timeout,
	}

	req = req.WithContext(ctx)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(resp.Body); err != nil {
			log.Errorf("failed reading error response body: %v", err)
		}
		return nil, fmt.Errorf("getting instance metadata status: %q, body: %q", resp.Status, buf.String())
	}

	out := &instanceMetadata{}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return nil, fmt.Errorf("decoding instance metadata: %w", err)
	}

	return out.Compute, nil
}
