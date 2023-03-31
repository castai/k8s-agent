package providers

import (
	"context"
	"os"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"castai-agent/internal/config"
	"castai-agent/internal/services/providers/eks"
	"castai-agent/internal/services/providers/gke"
	"castai-agent/internal/services/providers/kops"
	"castai-agent/internal/services/providers/openshift"
)

func TestGetProvider(t *testing.T) {
	t.Run("should return eks", func(t *testing.T) {
		r := require.New(t)

		t.Cleanup(config.Reset)
		t.Cleanup(os.Clearenv)

		r.NoError(os.Setenv("API_KEY", "api-key"))
		r.NoError(os.Setenv("API_URL", "test"))
		r.NoError(os.Setenv("PROVIDER", "eks"))
		r.NoError(os.Setenv("EKS_CLUSTER_NAME", "test"))
		r.NoError(os.Setenv("EKS_ACCOUNT_ID", "accountID"))
		r.NoError(os.Setenv("EKS_REGION", "eu-central-1"))

		got, err := GetProvider(context.Background(), logrus.New(), nil, nil)

		r.NoError(err)
		r.IsType(&eks.Provider{}, got)
	})

	t.Run("should return gke", func(t *testing.T) {
		r := require.New(t)

		t.Cleanup(config.Reset)
		t.Cleanup(os.Clearenv)

		r.NoError(os.Setenv("API_KEY", "api-key"))
		r.NoError(os.Setenv("API_URL", "test"))
		r.NoError(os.Setenv("PROVIDER", "gke"))
		r.NoError(os.Setenv("GKE_CLUSTER_NAME", "test"))
		r.NoError(os.Setenv("GKE_PROJECT_ID", "projectID"))
		r.NoError(os.Setenv("GKE_REGION", "us-east4"))

		got, err := GetProvider(context.Background(), logrus.New(), nil, nil)

		r.NoError(err)
		r.IsType(&gke.Provider{}, got)
	})

	t.Run("should return kops", func(t *testing.T) {
		r := require.New(t)

		t.Cleanup(config.Reset)
		t.Cleanup(os.Clearenv)

		r.NoError(os.Setenv("API_KEY", "api-key"))
		r.NoError(os.Setenv("API_URL", "test"))
		r.NoError(os.Setenv("PROVIDER", "kops"))

		got, err := GetProvider(context.Background(), logrus.New(), nil, nil)

		r.NoError(err)
		r.IsType(&kops.Provider{}, got)
	})

	t.Run("should return openshift", func(t *testing.T) {
		r := require.New(t)

		t.Cleanup(config.Reset)
		t.Cleanup(os.Clearenv)

		r.NoError(os.Setenv("API_KEY", "api-key"))
		r.NoError(os.Setenv("API_URL", "test"))
		r.NoError(os.Setenv("PROVIDER", "openshift"))

		got, err := GetProvider(context.Background(), logrus.New(), nil, nil)

		r.NoError(err)
		r.IsType(&openshift.Provider{}, got)
	})
}
