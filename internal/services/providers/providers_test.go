package providers

import (
	"context"
	"os"
	"testing"

	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"castai-agent/internal/config"
	"castai-agent/internal/services/providers/anywhere"
	"castai-agent/internal/services/providers/eks"
	"castai-agent/internal/services/providers/eks/aws"
	"castai-agent/internal/services/providers/gke"
	"castai-agent/internal/services/providers/kops"
	"castai-agent/internal/services/providers/openshift"
	"castai-agent/internal/services/providers/selfhostedec2"
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
		r.IsType(&aws.Provider{}, got)
		r.Equal(eks.Name, got.Name())
	})

	t.Run("should return selfhostedec2", func(t *testing.T) {
		r := require.New(t)

		t.Cleanup(config.Reset)
		t.Cleanup(os.Clearenv)

		r.NoError(os.Setenv("API_KEY", "api-key"))
		r.NoError(os.Setenv("API_URL", "test"))
		r.NoError(os.Setenv("PROVIDER", "selfhostedec2"))
		r.NoError(os.Setenv("SELFHOSTEDEC2_CLUSTER_NAME", "test"))
		r.NoError(os.Setenv("SELFHOSTEDEC2_ACCOUNT_ID", "accountID"))
		r.NoError(os.Setenv("SELFHOSTEDEC2_REGION", "eu-central-1"))

		got, err := GetProvider(context.Background(), logrus.New(), nil, nil)

		r.NoError(err)
		r.IsType(&aws.Provider{}, got)
		r.Equal(selfhostedec2.Name, got.Name())
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

	t.Run("should return anywhere", func(t *testing.T) {
		r := require.New(t)

		t.Cleanup(config.Reset)
		t.Cleanup(os.Clearenv)

		r.NoError(os.Setenv("API_KEY", "api-key"))
		r.NoError(os.Setenv("API_URL", "test"))
		r.NoError(os.Setenv("PROVIDER", "anywhere"))

		got, err := GetProvider(context.Background(), logrus.New(), nil, nil)

		r.NoError(err)
		r.IsType(&anywhere.Provider{}, got)
	})
}

func Test_isAPINodeLifecycleDiscoveryEnabled(t *testing.T) {
	tests := map[string]struct {
		cfg  config.Config
		want bool
	}{
		"should use default node lifecycle discovery value when EKS config is nil": {
			cfg:  config.Config{},
			want: true,
		},
		"should use default node lifecycle discovery value when EKS config is not nil and config value is nil": {
			cfg: config.Config{
				EKS: &config.EKS{APINodeLifecycleDiscoveryEnabled: nil},
			},
			want: true,
		},
		"should use node lifecycle discovery value from config when it is configured": {
			cfg: config.Config{
				EKS: &config.EKS{
					APINodeLifecycleDiscoveryEnabled: lo.ToPtr(false),
				},
			},
			want: false,
		},
	}

	for testName, tt := range tests {
		t.Run(testName, func(t *testing.T) {
			r := require.New(t)

			got := isAPINodeLifecycleDiscoveryEnabled(tt.cfg)

			r.Equal(tt.want, got)
		})
	}
}
