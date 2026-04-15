package monitor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/castai/logging/components"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/kubernetes"

	"castai-agent/internal/config"
	"castai-agent/internal/services/monitor"
	"castai-agent/pkg/castai"
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
		ExportConfig: components.Config{
			APIBaseURL:          cfg.API.URL,
			APIKey:              cfg.API.Key,
			Component:           "castai-agent-monitor",
			Version:             config.VersionInfo.Version,
			MaxRetries:          3,
			MaxRetryBackoffWait: 3 * time.Second,
		},
	}

	registrator := castai.NewRegistrator()
	defer registrator.ReleaseWaiters()

	logsExportManager := castailog.NewExporterManager()

	clusterIDHandler := func(clusterID string) error {
		loggingConfig.ExportConfig.ClusterID = clusterID
		log.Data["cluster_id"] = clusterID
		if err := logsExportManager.Setup(registrator, remoteLogger, localLog, &loggingConfig); err != nil {
			return err
		}
		registrator.ReleaseWaiters()
		return nil
	}

	errg, ctx := errgroup.WithContext(ctx)
	errg.Go(func() error {
		if err := logsExportManager.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("running logs exporter manager: %v", err)
		}
		return nil
	})

	errg.Go(func() error {
		err := runMonitorMode(ctx, log, cfg, clusterIDHandler)
		if err != nil {
			// it is necessary to log error because invoking of logrus exit handlers will terminate the process using os.Exit()
			// error handling in cobra ("github.com/spf13/cobra") won't be able to log this error
			remoteLogger.Error(err)
			return err
		}
		return nil
	})
	err := errg.Wait()
	defer castailog.InvokeLogrusExitHandlers(err)
	return err
}

func runMonitorMode(ctx context.Context, log *logrus.Entry, cfg config.Config, clusterIDChanged func(clusterID string) error) error {
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
