package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	require.NoError(t, os.Setenv("API_KEY", "abc"))
	require.NoError(t, os.Setenv("API_URL", "api.cast.ai"))

	require.NoError(t, os.Setenv("KUBECONFIG", "~/.kube/config"))

	require.NoError(t, os.Setenv("PROVIDER", "EKS"))

	require.NoError(t, os.Setenv("EKS_ACCOUNT_ID", "123"))
	require.NoError(t, os.Setenv("EKS_REGION", "eu-central-1"))
	require.NoError(t, os.Setenv("EKS_CLUSTER_NAME", "eks"))

	require.NoError(t, os.Setenv("CONTROLLER_PREP_TIMEOUT", "60m"))

	cfg := Get()

	require.Equal(t, "abc", cfg.API.Key)
	require.Equal(t, "https://api.cast.ai", cfg.API.URL)

	require.Equal(t, "~/.kube/config", cfg.Kubeconfig)

	require.Equal(t, "EKS", cfg.Provider)

	require.NotNil(t, cfg.EKS)
	require.Equal(t, "123", cfg.EKS.AccountID)
	require.Equal(t, "eu-central-1", cfg.EKS.Region)
	require.Equal(t, "eks", cfg.EKS.ClusterName)

	require.Equal(t, 60*time.Minute, cfg.Controller.PrepTimeout)
}
