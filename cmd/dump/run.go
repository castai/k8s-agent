package dump

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/metrics/pkg/client/clientset/versioned"

	"castai-agent/internal/config"
	"castai-agent/internal/services/controller"
	"castai-agent/internal/services/discovery"
	"castai-agent/internal/services/version"
)

func run(ctx context.Context) error {
	cfg := config.Get()

	logger := logrus.New()
	logger.SetLevel(logrus.Level(cfg.Log.Level))
	log := logger.WithField("version", config.VersionInfo.Version)

	log.Infof("starting dump of cluster snapshot")

	restconfig, err := cfg.RetrieveKubeConfig(log)
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(restconfig)
	if err != nil {
		return err
	}

	metricsClient, err := versioned.NewForConfig(restconfig)
	if err != nil {
		return fmt.Errorf("initializing metrics client: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(restconfig)
	if err != nil {
		return fmt.Errorf("initializing dynamic client: %w", err)
	}

	discoveryService := discovery.New(clientset, dynamicClient)

	var clusterID string
	if cfg.Static != nil && cfg.Static.ClusterID != "" {
		clusterID = cfg.Static.ClusterID
	} else {
		c, err := discoveryService.GetKubeSystemNamespaceID(ctx)
		if err != nil {
			return fmt.Errorf("getting cluster ID: %w", err)
		}
		clusterID = c.String()
	}

	v, err := version.Get(log, clientset)
	if err != nil {
		return fmt.Errorf("getting kubernetes version: %w", err)
	}

	log = log.WithField("k8s_version", v.Full())

	delta, err := controller.CollectSingleSnapshot(ctx, log, clusterID, clientset, dynamicClient, metricsClient, cfg.Controller, v, cfg.SelfPod.Namespace)
	if err != nil {
		return err
	}

	output := os.Stdout
	if out != "" {
		output, err = os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return err
		}
		defer output.Close()
	}

	if err := json.NewEncoder(output).Encode(delta); err != nil {
		return err
	}

	log.Infof("completed dump of cluster snapshot")

	return nil
}
