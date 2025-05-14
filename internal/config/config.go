package config

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Config struct {
	Log        Log    `mapstructure:"log"`
	API        API    `mapstructure:"api"`
	Kubeconfig string `mapstructure:"kubeconfig"`

	TLS *TLS `mapstructure:"tls"`

	Provider      string         `mapstructure:"provider"`
	EKS           *EKS           `mapstructure:"eks"`
	SelfHostedEC2 *SelfHostedEC2 `mapstructure:"selfhostedec2"`
	GKE           *GKE           `mapstructure:"gke"`
	KOPS          *KOPS          `mapstructure:"kops"`
	AKS           *AKS           `mapstructure:"aks"`
	OpenShift     *OpenShift     `mapstructure:"openshift"`
	Anywhere      *Anywhere      `mapstructure:"anywhere"`

	Static      *Static     `mapstructure:"static"`
	Controller  *Controller `mapstructure:"controller"`
	PprofPort   int         `mapstructure:"pprof_port"`
	HealthzPort int         `mapstructure:"healthz_port"`
	MetricsPort int         `mapstructure:"metrics_port"`

	MetadataStore *MetadataStoreConfig `mapstructure:"metadata_store"`

	LeaderElection LeaderElectionConfig `mapstructure:"leader_election"`

	MonitorMetadata string `mapstructure:"monitor_metadata"`
	SelfPod         Pod    `mapstructure:"self_pod"`
}

const (
	DefaultAPINodeLifecycleDiscoveryEnabled = true

	LogExporterSendTimeout = 15 * time.Second
)

type TLS struct {
	CACertFile string `mapstructure:"ca_cert_file"`
}

type Log struct {
	Level                 int            `mapstructure:"level"`
	ExporterSenderTimeout time.Duration  `mapstructure:"exporter_timeout"`
	PrintMemoryUsageEvery *time.Duration `mapstructure:"print_memory_usage_every"`
}

type Pod struct {
	Namespace string `mapstructure:"namespace"`
	Name      string `mapstructure:"name"`
	Node      string `mapstructure:"node"`
}

type API struct {
	Key                   string        `mapstructure:"key"`
	URL                   string        `mapstructure:"url"`
	HostHeaderOverride    string        `mapstructure:"host_header_override"`
	Timeout               time.Duration `mapstructure:"timeout"`
	DeltaReadTimeout      time.Duration `mapstructure:"delta_read_timeout"`
	TotalSendDeltaTimeout time.Duration `mapstructure:"total_send_delta_timeout"`
}

type EKS struct {
	AccountID                        string        `mapstructure:"account_id"`
	Region                           string        `mapstructure:"region"`
	ClusterName                      string        `mapstructure:"cluster_name"`
	APITimeout                       time.Duration `mapstructure:"api_timeout"`
	APINodeLifecycleDiscoveryEnabled *bool         `mapstructure:"api_node_lifecycle_discovery_enabled"`
}

type SelfHostedEC2 struct {
	AccountID                        string        `mapstructure:"account_id"`
	Region                           string        `mapstructure:"region"`
	ClusterName                      string        `mapstructure:"cluster_name"`
	APITimeout                       time.Duration `mapstructure:"api_timeout"`
	APINodeLifecycleDiscoveryEnabled *bool         `mapstructure:"api_node_lifecycle_discovery_enabled"`
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
	ClusterID string `mapstructure:"cluster_id"`
}

type MetadataStoreConfig struct {
	Enabled            bool   `mapstructure:"enabled"`
	ConfigMapNamespace string `mapstructure:"config_map_namespace"`
	ConfigMapName      string `mapstructure:"config_map_name"`
}

type Anywhere struct {
	ClusterName string `mapstructure:"cluster_name"`
}

type Controller struct {
	Interval                       time.Duration `mapstructure:"interval"`
	MemoryPressureInterval         time.Duration `mapstructure:"memory_pressure_interval"`
	PrepTimeout                    time.Duration `mapstructure:"prep_timeout"`
	InitialSleepDuration           time.Duration `mapstructure:"initial_sleep_duration"`
	HealthySnapshotIntervalLimit   time.Duration `mapstructure:"healthy_snapshot_interval_limit"`
	InitializationTimeoutExtension time.Duration `mapstructure:"initialization_timeout_extension"`
	ConfigMapNamespaces            []string      `mapstructure:"config_map_namespaces"`
	RemoveAnnotationsPrefixes      []string      `mapstructure:"remove_annotations_prefixes"`
	AnnotationsMaxLength           string        `mapstructure:"annotations_max_length"`
	ForcePagination                bool          `mapstructure:"force_pagination"`
	PageSize                       int64         `mapstructure:"page_size"`
	FilterEmptyReplicaSets         bool          `mapstructure:"filter_empty_replica_sets"`

	// DisabledInformers contains a list of informers to disable,
	// for example:
	//	*v1.Event:oom,*v1.Deployment
	// Note that appsv1 is referenced as v1 and the asterisk (*)
	DisabledInformers []string `mapstructure:"disabled_informers"`
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

	viper.SetDefault("api.timeout", 10*time.Second)
	viper.SetDefault("api.delta_read_timeout", 2*time.Minute)
	viper.SetDefault("api.total_send_delta_timeout", 5*time.Minute)

	viper.SetDefault("controller.interval", 15*time.Second)
	viper.SetDefault("controller.prep_timeout", 10*time.Minute)
	viper.SetDefault("controller.memory_pressure_interval", 3*time.Second)
	viper.SetDefault("controller.initial_sleep_duration", 30*time.Second)
	viper.SetDefault("controller.healthy_snapshot_interval_limit", 12*time.Minute)
	viper.SetDefault("controller.initialization_timeout_extension", 5*time.Minute)
	viper.SetDefault("controller.force_pagination", false)
	viper.SetDefault("controller.page_size", 500)

	viper.SetDefault("healthz_port", 9876)
	viper.SetDefault("metrics_port", 9877)
	viper.SetDefault("leader_election.enabled", true)
	viper.SetDefault("leader_election.lock_name", "agent-leader-election-lock")
	viper.SetDefault("leader_election.namespace", "castai-agent")

	viper.SetDefault("metadata_store.enabled", false)
	viper.SetDefault("metadata_store.config_map_name", "castai-agent-metadata")
	viper.SetDefault("metadata_store.config_map_namespace", "castai-agent")

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AllowEmptyEnv(true)

	cfg = &Config{}
	bindEnvs(*cfg)

	if cfgFile := os.Getenv("CONFIG_PATH"); cfgFile != "" {
		fmt.Println("Using config from a file", cfgFile)
		viper.SetConfigType("yaml")
		viper.SetConfigFile(cfgFile)
		if err := viper.ReadInConfig(); err != nil {
			panic(fmt.Errorf("reading default config: %v", err))
		}
	}

	if err := viper.Unmarshal(&cfg); err != nil {
		panic(fmt.Errorf("parsing configuration: %v", err))
	}

	if cfg.Log.Level == 0 {
		cfg.Log.Level = int(logrus.InfoLevel)
	}
	if cfg.Log.ExporterSenderTimeout == 0 {
		cfg.Log.ExporterSenderTimeout = LogExporterSendTimeout
	}

	if !strings.HasPrefix(cfg.API.URL, "https://") && !strings.HasPrefix(cfg.API.URL, "http://") {
		cfg.API.URL = fmt.Sprintf("https://%s", cfg.API.URL)
	}
	cfg.API.URL = strings.TrimSuffix(cfg.API.URL, "/")

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
		if cfg.EKS.APITimeout <= 0 {
			cfg.EKS.APITimeout = 120 * time.Second
		}
		if cfg.EKS.APINodeLifecycleDiscoveryEnabled == nil {
			cfg.EKS.APINodeLifecycleDiscoveryEnabled = lo.ToPtr(DefaultAPINodeLifecycleDiscoveryEnabled)
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

func (c *Config) RetrieveKubeConfig(log logrus.FieldLogger) (*rest.Config, error) {
	kubeconfig, err := kubeConfigFromPath(cfg.Kubeconfig)
	if err != nil {
		return nil, err
	}

	if kubeconfig != nil {
		log.Debug("using kubeconfig from env variables")
		return kubeconfig, nil
	}

	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	log.Debug("using in cluster kubeconfig")
	return inClusterConfig, nil
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

func kubeConfigFromPath(kubepath string) (*rest.Config, error) {
	if kubepath == "" {
		return nil, nil
	}

	data, err := os.ReadFile(kubepath)
	if err != nil {
		return nil, fmt.Errorf("reading kubeconfig at %s: %w", kubepath, err)
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(data)
	if err != nil {
		return nil, fmt.Errorf("building rest config from kubeconfig at %s: %w", kubepath, err)
	}

	return restConfig, nil
}
