package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	r := require.New(t)
	r.NoError(os.Setenv("API_KEY", "abc"))
	r.NoError(os.Setenv("API_URL", "api.cast.ai"))

	r.NoError(os.Setenv("KUBECONFIG", "~/.kube/config"))

	r.NoError(os.Setenv("PROVIDER", "EKS"))

	r.NoError(os.Setenv("EKS_ACCOUNT_ID", "123"))
	r.NoError(os.Setenv("EKS_REGION", "eu-central-1"))
	r.NoError(os.Setenv("EKS_CLUSTER_NAME", "eks"))

	cfg := Get()

	r.Equal(9876, cfg.HealthzPort)
	r.Equal("agent-leader-election-lock", cfg.LeaderElection.LockName)
	r.Equal("castai-agent", cfg.LeaderElection.Namespace)
	r.Equal("abc", cfg.API.Key)
	r.Equal("https://api.cast.ai", cfg.API.URL)

	r.Equal("~/.kube/config", cfg.Kubeconfig)

	r.Equal("EKS", cfg.Provider)

	r.NotNil(cfg.EKS)
	r.Equal("123", cfg.EKS.AccountID)
	r.Equal("eu-central-1", cfg.EKS.Region)
	r.Equal("eks", cfg.EKS.ClusterName)
}
