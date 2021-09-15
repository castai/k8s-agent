package config

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type Config struct {
	Log        Log
	API        API
	Kubeconfig string
	Provider   string
	CASTAI     *CASTAI
	EKS        *EKS
	GKE        *GKE
	KOPS       *KOPS
}

type Log struct {
	Level int
}

type API struct {
	Key string
	URL string
}

type CASTAI struct {
	ClusterID      string
	OrganizationID string
}

type EKS struct {
	AccountID   string
	Region      string
	ClusterName string
}

type GKE struct {
	Region      string
	ProjectID   string
	ClusterName string
	Location    string
}

type KOPS struct {
	CSP         string
	Region      string
	ClusterName string
	StateStore  string
}

// Get configuration bound to environment variables.
func Get() *Config {
	_ = viper.BindEnv("log.level", "LOG_LEVEL")

	_ = viper.BindEnv("api.key", "API_KEY")
	_ = viper.BindEnv("api.url", "API_URL")

	_ = viper.BindEnv("kubeconfig")

	_ = viper.BindEnv("provider")

	_ = viper.BindEnv("castai.clusterid", "CASTAI_CLUSTER_ID")
	_ = viper.BindEnv("castai.organizationid", "CASTAI_ORGANIZATION_ID")

	_ = viper.BindEnv("eks.accountid", "EKS_ACCOUNT_ID")
	_ = viper.BindEnv("eks.region", "EKS_REGION")
	_ = viper.BindEnv("eks.clustername", "EKS_CLUSTER_NAME")

	_ = viper.BindEnv("gke.region", "GKE_REGION")
	_ = viper.BindEnv("gke.projectid", "GKE_PROJECT_ID")
	_ = viper.BindEnv("gke.clustername", "GKE_CLUSTER_NAME")
	_ = viper.BindEnv("gke.location", "GKE_LOCATION")

	_ = viper.BindEnv("kops.csp", "KOPS_CSP")
	_ = viper.BindEnv("kops.region", "KOPS_REGION")
	_ = viper.BindEnv("kops.clustername", "KOPS_CLUSTER_NAME")
	_ = viper.BindEnv("kops.statestore", "KOPS_STATE_STORE")

	cfg := &Config{}
	if err := viper.Unmarshal(&cfg); err != nil {
		panic(fmt.Errorf("parsing configuration: %v", err))
	}

	if cfg.Log.Level == 0 {
		cfg.Log.Level = int(logrus.InfoLevel)
	}

	if cfg.API.Key == "" {
		required("API_KEY")
	}
	if cfg.API.URL == "" {
		required("API_URL")
	}

	if cfg.Provider == ProviderCASTAI && cfg.CASTAI == nil {
		cfg.CASTAI = &CASTAI{}
	}

	if cfg.CASTAI != nil {
		if cfg.CASTAI.ClusterID == "" {
			requiredWhenDiscoveryDisabled("CASTAI_CLUSTER_ID")
		}
		if cfg.CASTAI.OrganizationID == "" {
			requiredWhenDiscoveryDisabled("CASTAI_ORGANIZATION_ID")
		}
	}

	if cfg.EKS != nil {
		if cfg.EKS.AccountID == "" {
			requiredWhenDiscoveryDisabled("EKS_ACCOUNT_ID")
		}
		if cfg.EKS.Region == "" {
			requiredWhenDiscoveryDisabled("EKS_REGION")
		}
		if cfg.EKS.ClusterName == "" {
			requiredWhenDiscoveryDisabled("EKS_CLUSTER_NAME")
		}
	}

	if cfg.KOPS != nil {
		if cfg.KOPS.CSP == "" {
			requiredWhenDiscoveryDisabled("KOPS_CSP")
		}
		if cfg.KOPS.Region == "" {
			requiredWhenDiscoveryDisabled("KOPS_REGION")
		}
		if cfg.KOPS.ClusterName == "" {
			requiredWhenDiscoveryDisabled("KOPS_CLUSTER_NAME")
		}
		if cfg.KOPS.StateStore == "" {
			requiredWhenDiscoveryDisabled("KOPS_STATE_STORE")
		}
	}

	return cfg
}

func required(variable string) {
	panic(fmt.Errorf("env variable %s is required", variable))
}

func requiredWhenDiscoveryDisabled(variable string) {
	panic(fmt.Errorf("env variable %s is required when discovery is disabled", variable))
}
