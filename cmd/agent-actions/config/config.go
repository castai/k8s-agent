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
	ClusterID  string
	PprofPort  int
}

type Log struct {
	Level int
}

type API struct {
	Key          string
	URL          string
	TelemetryURL string
}

var cfg *Config

// Get configuration bound to environment variables.
func Get() Config {
	if cfg != nil {
		return *cfg
	}

	_ = viper.BindEnv("log.level", "LOG_LEVEL")

	_ = viper.BindEnv("api.key", "API_KEY")
	_ = viper.BindEnv("api.url", "API_URL")
	_ = viper.BindEnv("api.telemetryurl", "API_TELEMETRY_URL")
	_ = viper.BindEnv("clusterid", "CLUSTER_ID")

	_ = viper.BindEnv("kubeconfig")

	_ = viper.BindEnv("pprofport", "PPROF_PORT")

	cfg = &Config{}
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
	if cfg.API.TelemetryURL == "" {
		required("API_TELEMETRY_URL")
	}
	if cfg.ClusterID == "" {
		required("CLUSTER_ID")
	}

	return *cfg
}

func required(variable string) {
	panic(fmt.Errorf("env variable %s is required", variable))
}
