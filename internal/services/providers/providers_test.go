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
)

func TestGetProvider(t *testing.T) {
	t.Run("should return castai", func(t *testing.T) {
		t.Cleanup(config.Reset)

		require.NoError(t, os.Setenv("API_KEY", "api-key"))
		require.NoError(t, os.Setenv("API_URL", "test"))
		require.NoError(t, os.Setenv("PROVIDER", "castai"))

		got, err := GetProvider(context.Background(), logrus.New())

		require.NoError(t, err)
		require.IsType(t, &castai.Provider{}, got)
	})

	t.Run("should return eks", func(t *testing.T) {
		t.Cleanup(config.Reset)

		require.NoError(t, os.Setenv("API_KEY", "api-key"))
		require.NoError(t, os.Setenv("API_URL", "test"))
		require.NoError(t, os.Setenv("PROVIDER", "eks"))
		require.NoError(t, os.Setenv("EKS_CLUSTER_NAME", "test"))
		require.NoError(t, os.Setenv("EKS_ACCOUNT_ID", "accountID"))
		require.NoError(t, os.Setenv("EKS_REGION", "eu-central-1"))

		got, err := GetProvider(context.Background(), logrus.New())

		require.NoError(t, err)
		require.IsType(t, &eks.Provider{}, got)
	})
}
