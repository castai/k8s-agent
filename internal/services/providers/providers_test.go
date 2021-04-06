package providers

import (
	"castai-agent/internal/services/providers/eks"
	"context"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestGetProvider(t *testing.T) {
	require.NoError(t, os.Setenv("API_KEY", "api-key"))
	require.NoError(t, os.Setenv("API_URL", "test"))

	t.Run("should return eks", func(t *testing.T) {
		require.NoError(t, os.Setenv("PROVIDER", "eks"))
		require.NoError(t, os.Setenv("EKS_CLUSTER_NAME", "test"))
		require.NoError(t, os.Setenv("EKS_ACCOUNT_ID", "accountID"))
		require.NoError(t, os.Setenv("EKS_REGION", "eu-central-1"))

		p, err := GetProvider(context.Background(), logrus.New())

		require.NoError(t, err)
		require.IsType(t, &eks.Provider{}, p)
	})
}
