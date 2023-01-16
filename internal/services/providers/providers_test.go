package providers

import (
	"context"
	"os"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"castai-agent/internal/config"
	"castai-agent/internal/services/providers/castai"
	"castai-agent/internal/services/providers/eks"
	"castai-agent/internal/services/providers/gke"
	"castai-agent/internal/services/providers/kops"
)

func TestGetProvider(t *testing.T) {
	t.Run("should return castai", func(t *testing.T) {
		t.Cleanup(config.Reset)
		t.Cleanup(os.Clearenv)

		require.NoError(t, os.Setenv("API_KEY", "api-key"))
		require.NoError(t, os.Setenv("API_URL", "test"))
		require.NoError(t, os.Setenv("PROVIDER", "castai"))

		got, err := GetProvider(context.Background(), logrus.New(), nil, nil)

		require.NoError(t, err)
		require.IsType(t, &castai.Provider{}, got)
	})

	t.Run("should return eks", func(t *testing.T) {
		t.Cleanup(config.Reset)
		t.Cleanup(os.Clearenv)

		require.NoError(t, os.Setenv("API_KEY", "api-key"))
		require.NoError(t, os.Setenv("API_URL", "test"))
		require.NoError(t, os.Setenv("PROVIDER", "eks"))
		require.NoError(t, os.Setenv("EKS_CLUSTER_NAME", "test"))
		require.NoError(t, os.Setenv("EKS_ACCOUNT_ID", "accountID"))
		require.NoError(t, os.Setenv("EKS_REGION", "eu-central-1"))

		got, err := GetProvider(context.Background(), logrus.New(), nil, nil)

		require.NoError(t, err)
		require.IsType(t, &eks.Provider{}, got)
	})

	t.Run("should return gke", func(t *testing.T) {
		t.Cleanup(config.Reset)
		t.Cleanup(os.Clearenv)

		require.NoError(t, os.Setenv("API_KEY", "api-key"))
		require.NoError(t, os.Setenv("API_URL", "test"))
		require.NoError(t, os.Setenv("PROVIDER", "gke"))
		require.NoError(t, os.Setenv("GKE_CLUSTER_NAME", "test"))
		require.NoError(t, os.Setenv("GKE_PROJECT_ID", "projectID"))
		require.NoError(t, os.Setenv("GKE_REGION", "us-east4"))

		got, err := GetProvider(context.Background(), logrus.New(), nil, nil)

		require.NoError(t, err)
		require.IsType(t, &gke.Provider{}, got)
	})

	t.Run("should return kops", func(t *testing.T) {
		t.Cleanup(config.Reset)
		t.Cleanup(os.Clearenv)

		require.NoError(t, os.Setenv("API_KEY", "api-key"))
		require.NoError(t, os.Setenv("API_URL", "test"))
		require.NoError(t, os.Setenv("PROVIDER", "kops"))

		got, err := GetProvider(context.Background(), logrus.New(), nil, nil)

		require.NoError(t, err)
		require.IsType(t, &kops.Provider{}, got)
	})
}
