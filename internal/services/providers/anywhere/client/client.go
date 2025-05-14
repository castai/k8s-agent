//go:generate mockgen -destination ./mock/client.go . Client
package client

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	"github.com/sirupsen/logrus"

	"castai-agent/internal/services/discovery"
	eks_client "castai-agent/internal/services/providers/eks/aws"
	gke_client "castai-agent/internal/services/providers/gke/client"
	"castai-agent/pkg/cloud"
)

type Client interface {
	// GetClusterName attempts to discover the name of the cluster.
	GetClusterName(ctx context.Context) (string, error)
}

type client struct {
	log               logrus.FieldLogger
	discoveryService  discovery.Service
	eksClient         eks_client.Client
	gkeMetadataClient gke_client.Metadata
}

func New(log logrus.FieldLogger, discoveryService discovery.Service) Client {
	return &client{
		log:              log,
		discoveryService: discoveryService,
	}
}

func (c *client) GetClusterName(ctx context.Context) (string, error) {
	csp, _ := c.discoveryService.GetCSP(ctx)
	if csp == cloud.AWS {
		if c.eksClient == nil {
			client, err := eks_client.New(ctx, c.log, eks_client.WithEC2Client(), eks_client.WithMetadataDiscovery())

			if err != nil {
				return "", err
			}

			c.eksClient = client
		}

		if c.eksClient != nil {
			clusterName, err := c.eksClient.GetClusterName(ctx)

			return lo.FromPtrOr(clusterName, ""), err
		}
	} else if csp == cloud.GCP {
		if c.gkeMetadataClient == nil {
			c.gkeMetadataClient = gke_client.NewMetadataClient()
		}

		if c.gkeMetadataClient != nil {
			return c.gkeMetadataClient.GetClusterName()
		}
	}

	return "", fmt.Errorf("cluster name could not be determined automatically")
}
