//go:generate mockgen -destination ./mock/client.go . Metadata
package client

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/compute/metadata"
)

type Metadata interface {
	// GetProjectID returns GCP projectID GKE cluster is running in.
	GetProjectID() (string, error)
	// GetRegion returns GCP region GKE cluster is running in.
	GetRegion() (string, error)
	// GetLocation returns GKE cluster location. Location identifies where cluster control plane is running. Zone or Region.
	GetLocation() (string, error)
	// GetClusterName returns GKE cluster name.
	GetClusterName() (string, error)
}

type gcp struct {
	metadata *metadata.Client
}

func NewMetadataClient() Metadata {
	metaClient := metadata.NewClient(&http.Client{
		Timeout: time.Second * 10,
	})

	return &gcp{metadata: metaClient}
}

func (g *gcp) GetProjectID() (string, error) {
	return g.metadata.ProjectID()
}

func (g *gcp) GetRegion() (string, error) {
	zone, err := g.metadata.Zone()
	if err != nil {
		return "", err
	}

	return regionFromZone(zone)
}

func regionFromZone(zone string) (string, error) {
	if zone == "" {
		return "", errors.New("given zone input is empty")
	}

	parts := strings.Split(zone, "-")
	if len(parts) != 3 {
		return "", fmt.Errorf("cannot parse provided zone %q to region", zone)
	}

	region := fmt.Sprintf("%s-%s", parts[0], parts[1])

	return region, nil
}

func (g *gcp) GetLocation() (string, error) {
	return g.metadata.InstanceAttributeValue("cluster-location")
}

func (g *gcp) GetClusterName() (string, error) {
	return g.metadata.InstanceAttributeValue("cluster-name")
}
