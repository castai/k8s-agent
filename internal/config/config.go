package config

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type Config struct {
	Log        Log    `mapstructure:"log"`
	Mode       Mode   `mapstructure:"mode"`
	API        API    `mapstructure:"api"`
	Kubeconfig string `mapstructure:"kubeconfig"`

	Provider  string     `mapstructure:"provider"`
	EKS       *EKS       `mapstructure:"eks"`
	GKE       *GKE       `mapstructure:"gke"`
	KOPS      *KOPS      `mapstructure:"kops"`
	AKS       *AKS       `mapstructure:"aks"`
	OpenShift *OpenShift `mapstructure:"openshift"`

	Static      *Static     `mapstructure:"static"`
	Controller  *Controller `mapstructure:"controller"`
	PprofPort   int         `mapstructure:"pprof.port"`
	HealthzPort int         `mapstructure:"healthz_port"`

	LeaderElection LeaderElectionConfig `mapstructure:"leader_election"`

	MonitorMetadata string `mapstructure:"monitor_metadata"`
	SelfPod         Pod    `mapstructure:"self_pod"`
}

type Mode string

const (
	ModeAgent   Mode = "agent"
	ModeMonitor Mode = "monitor"
)

type Log struct {
	Level int `mapstructure:"level"`
}

type Pod struct {
	Namespace string `mapstructure:"namespace"`
	Name      string `mapstructure:"name"`
	Node      string `mapstructure:"node"`
}

type API struct {
	Key string `mapstructure:"key"`
	URL string `mapstructure:"url"`
}

type EKS struct {
	AccountID   string `mapstructure:"account_id"`
	Region      string `mapstructure:"region"`
	ClusterName string `mapstructure:"cluster_name"`
}

type GKE struct {
	Region      string `mapstructure:"region"`
	ProjectID   string `mapstructure:"project_id"`
	ClusterName string `mapstructure:"cluster_name"`
	Location    string `mapstructure:"location"`
}

type KOPS struct {
	CSP         string `mapstructure:"csp"`
	Region      string `mapstructure:"region"`
	ClusterName string `mapstructure:"cluster_name"`
	StateStore  string `mapstructure:"state_store"`
}

type AKS struct {
	NodeResourceGroup string `mapstructure:"node_resource_group"`
	Location          string `mapstructure:"location"`
	SubscriptionID    string `mapstructure:"subscription_id"`
}

type OpenShift struct {
	CSP         string `mapstructure:"csp"`
	Region      string `mapstructure:"region"`
	ClusterName string `mapstructure:"cluster_name"`
	InternalID  string `mapstructure:"internal_id"`
}

type Static struct {
	SkipClusterRegistration bool   `mapstructure:"skip_cluster_registration"`
	ClusterID               string `mapstructure:"cluster_id"`
	OrganizationID          string `mapstructure:"organization_id"`
}

type Controller struct {
	Interval                       time.Duration `mapstructure:"interval"`
	PrepTimeout                    time.Duration `mapstructure:"prep_timeout"`
	InitialSleepDuration           time.Duration `mapstructure:"initial_sleep_duration"`
	HealthySnapshotIntervalLimit   time.Duration `mapstructure:"healthy_snapshot_interval_limit"`
	InitializationTimeoutExtension time.Duration `mapstructure:"initialization_timeout_extension"`
}

type LeaderElectionConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	LockName  string `mapstructure:"lock_name"`
	Namespace string `mapstructure:"namespace"`
}

var cfg *Config
var mu sync.Mutex

// Get configuration bound to environment variables.
func Get() Config {
	if cfg != nil {
		return *cfg
	}

	mu.Lock()
	defer mu.Unlock()
	if cfg != nil {
		return *cfg
	}

	viper.SetDefault("mode", ModeAgent)

	viper.SetDefault("controller.interval", 15*time.Second)
	viper.SetDefault("controller.prep_timeout", 10*time.Minute)
	viper.SetDefault("controller.initial_sleep_duration", 30*time.Second)
	viper.SetDefault("controller.healthy_snapshot_interval_limit", 12*time.Minute)
	viper.SetDefault("controller.initialization_timeout_extension", 5*time.Minute)

	viper.SetDefault("healthz_port", 9876)
	viper.SetDefault("leader_election.enabled", true)
	viper.SetDefault("leader_election.lock_name", "agent-leader-election-lock")
	viper.SetDefault("leader_election.namespace", "castai-agent")

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AllowEmptyEnv(true)

	cfg = &Config{}
	bindEnvs(*cfg)
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

	if !strings.HasPrefix(cfg.API.URL, "https://") && !strings.HasPrefix(cfg.API.URL, "http://") {
		cfg.API.URL = fmt.Sprintf("https://%s", cfg.API.URL)
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

	if cfg.AKS != nil {
		if cfg.AKS.SubscriptionID == "" {
			requiredWhenDiscoveryDisabled("AKS_SUBSCRIPTION_ID")
		}
		if cfg.AKS.Location == "" {
			requiredWhenDiscoveryDisabled("AKS_LOCATION")
		}
		if cfg.AKS.NodeResourceGroup == "" {
			requiredWhenDiscoveryDisabled("AKS_NODE_RESOURCE_GROUP")
		}
	}

	if cfg.OpenShift != nil {
		if cfg.OpenShift.CSP == "" {
			requiredWhenDiscoveryDisabled("OPENSHIFT_CSP")
		}
		if cfg.OpenShift.Region == "" {
			requiredWhenDiscoveryDisabled("OPENSHIFT_REGION")
		}
		if cfg.OpenShift.ClusterName == "" {
			requiredWhenDiscoveryDisabled("OPENSHIFT_CLUSTER_NAME")
		}
		if cfg.OpenShift.InternalID == "" {
			requiredWhenDiscoveryDisabled("OPENSHIFT_INTERNAL_ID")
		}
	}

	return *cfg
}

// Reset is used only for unit testing to reset configuration and rebind variables.
func Reset() {
	cfg = nil
}

func required(variable string) {
	panic(fmt.Errorf("env variable %s is required", variable))
}

func requiredWhenDiscoveryDisabled(variable string) {
	panic(fmt.Errorf("env variable %s is required when discovery is disabled", variable))
}
