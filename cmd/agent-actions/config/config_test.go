package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	require.NoError(t, os.Setenv("API_KEY", "abc"))
	require.NoError(t, os.Setenv("API_URL", "api.cast.ai"))
	require.NoError(t, os.Setenv("API_TELEMETRY_URL", "telemetry.cast.ai"))
	require.NoError(t, os.Setenv("KUBECONFIG", "~/.kube/config"))
	require.NoError(t, os.Setenv("CLUSTER_ID", "c1"))
	require.NoError(t, os.Setenv("PPROF_PORT", "6060"))

	cfg := Get()

	require.Equal(t, "abc", cfg.API.Key)
	require.Equal(t, "api.cast.ai", cfg.API.URL)
	require.Equal(t, "telemetry.cast.ai", cfg.API.TelemetryURL)
	require.Equal(t, "~/.kube/config", cfg.Kubeconfig)
	require.Equal(t, "c1", cfg.ClusterID)
	require.Equal(t, 6060, cfg.PprofPort)
}
