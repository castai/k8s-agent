package monitor

import (
	"context"
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/monitor"
	castailog "castai-agent/pkg/log"
)

func run(ctx context.Context) error {
	cfg := config.Get()
	if cfg.API.Key == "" {
		return errors.New("env variable \"API_KEY\" is required")
	}
	if cfg.API.URL == "" {
		return errors.New("env variable \"API_URL\" is required")
	}

	remoteLogger := logrus.New()
	remoteLogger.SetLevel(logrus.Level(cfg.Log.Level))
	log := remoteLogger.WithField("version", config.VersionInfo.Version)

	localLog := logrus.New()
	localLog.SetLevel(logrus.DebugLevel)

	loggingConfig := castailog.Config{
		SendTimeout: cfg.Log.ExporterSenderTimeout,
	}

	restyClient, err := castai.NewDefaultRestyClient()
	if err != nil {
		log.Fatalf("resty client failed: %v", err)
	}

	deltaHTTPClient, err := castai.NewDefaultDeltaHTTPClient()
	if err != nil {
		log.Fatalf("delta http client failed: %v", err)
	}

	castaiClient := castai.NewClient(log, restyClient, deltaHTTPClient)

	registrator := castai.NewRegistrator()
	defer registrator.ReleaseWaiters()

	castailog.SetupLogExporter(registrator, remoteLogger, localLog, castaiClient, &loggingConfig)
	clusterIDHandler := func(clusterID string) {
		loggingConfig.ClusterID = clusterID
		log.Data["cluster_id"] = clusterID
		registrator.ReleaseWaiters()
	}

	err = runMonitorMode(ctx, log, cfg, clusterIDHandler)
	if err != nil {
		// it is necessary to log error because invoking of logrus exit handlers will terminate the process using os.Exit()
		// error handling in cobra ("github.com/spf13/cobra") won't be able to log this error
		remoteLogger.Error(err)
	}
	defer castailog.InvokeLogrusExitHandlers(err)
	return err
}

func runMonitorMode(ctx context.Context, log *logrus.Entry, cfg config.Config, clusterIDChanged func(clusterID string)) error {
	restconfig, err := cfg.RetrieveKubeConfig(log)
	if err != nil {
		return fmt.Errorf("retrieving kubeconfig: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(restconfig)
	if err != nil {
		return fmt.Errorf("obtaining kubernetes clientset: %w", err)
	}

	return monitor.Run(ctx, log, clientset, cfg.MonitorMetadata, cfg.SelfPod, clusterIDChanged)
}
