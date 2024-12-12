package config

import (
	"os"
	"testing"

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
	require.NoError(t, os.Setenv("PPROF_PORT", "6060"))

	cfg := Get()

	require.Equal(t, cfg.HealthzPort, 9876)
	require.Equal(t, cfg.LeaderElection.LockName, "agent-leader-election-lock")
	require.Equal(t, cfg.LeaderElection.Namespace, "castai-agent")
	require.Equal(t, "abc", cfg.API.Key)
	require.Equal(t, "https://api.cast.ai", cfg.API.URL)
	require.Equal(t, 6060, cfg.PprofPort)
	require.Equal(t, "~/.kube/config", cfg.Kubeconfig)

	require.Equal(t, "EKS", cfg.Provider)

	require.NotNil(t, cfg.EKS)
	require.Equal(t, "123", cfg.EKS.AccountID)
	require.Equal(t, "eu-central-1", cfg.EKS.Region)
	require.Equal(t, "eks", cfg.EKS.ClusterName)
}
