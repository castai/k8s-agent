package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	metadataURL = "http://169.254.169.254/metadata/instance"
	timeout     = 2 * time.Second
)

func NewClient() Client {
	return Client{}
}

type Client struct {
	metadata *ComputeMetadata
}

func (c *Client) GetLocation(ctx context.Context) (string, error) {
	if c.metadata == nil {
		md, err := getInstanceMetadata(ctx)
		if err != nil {
			return "", err
		}
		c.metadata = md
	}
	return c.metadata.Location, nil
}

func (c *Client) GetResourceGroup(ctx context.Context) (string, error) {
	if c.metadata == nil {
		md, err := getInstanceMetadata(ctx)
		if err != nil {
			return "", err
		}
		c.metadata = md
	}
	return c.metadata.ResourceGroup, nil
}

func (c *Client) GetSubscriptionID(ctx context.Context) (string, error) {
	if c.metadata == nil {
		md, err := getInstanceMetadata(ctx)
		if err != nil {
			return "", err
		}
		c.metadata = md
	}
	return c.metadata.SubscriptionID, nil
}


type instanceMetadata struct {
	Compute *ComputeMetadata `json:"compute,omitempty"`
}

type ComputeMetadata struct {
	Location       string `json:"location"`
	ResourceGroup  string `json:"resourceGroupName"`
	SubscriptionID string `json:"subscriptionId"`
}

func getInstanceMetadata(ctx context.Context) (*ComputeMetadata, error) {
	req, err := http.NewRequest("GET", metadataURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Metadata", "True")
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
		return nil, fmt.Errorf("getting instance metadata with response %q", resp.Status)
	}

	out := &instanceMetadata{}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return nil, fmt.Errorf("decoding instance metadata: %w", err)
	}

	return out.Compute, nil
}
