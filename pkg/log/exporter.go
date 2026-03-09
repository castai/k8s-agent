package log

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/castai/logging/components"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"

	"castai-agent/pkg/castai"
)

type Exporter interface {
	logrus.Hook
	Wait()
}

func NewExporterManager() *ExporterManager {
	return &ExporterManager{}
}

type ExporterManager struct {
	ingestClientBatchClient ingestClient
}

func (m *ExporterManager) Setup(registrator *castai.Registrator, logger *logrus.Logger, localLog logrus.FieldLogger, cfg *Config) error {
	ingestClient, err := components.NewAPIClient(cfg.ExportConfig)
	if err != nil {
		return err
	}
	ingestClientBatchClient := components.NewBatchClient(ingestClient, components.BatchSize(500))

	return m.setup(ingestClientBatchClient, registrator, logger, localLog, cfg)
}

func (m *ExporterManager) setup(ingestClientBatchClient ingestClient, registrator *castai.Registrator, logger *logrus.Logger, localLog logrus.FieldLogger, cfg *Config) error {
	logExporter, err := newExporter(registrator, cfg, localLog, ingestClientBatchClient)
	if err != nil {
		return err
	}
	logger.AddHook(logExporter)
	logrus.RegisterExitHandler(logExporter.Wait)
	m.ingestClientBatchClient = ingestClientBatchClient
	return nil
}

func (m *ExporterManager) Run(ctx context.Context) error {
	return m.ingestClientBatchClient.Run(ctx)
}

func newExporter(registrator *castai.Registrator, cfg *Config, localLog logrus.FieldLogger, ingestClientBatchClient ingestClient) (Exporter, error) {
	return &exporter{
		registrator:             registrator,
		cfg:                     cfg,
		localLog:                localLog,
		wg:                      sync.WaitGroup{},
		ingestClientBatchClient: ingestClientBatchClient,
	}, nil
}

type exporter struct {
	registrator             *castai.Registrator
	localLog                logrus.FieldLogger
	cfg                     *Config
	wg                      sync.WaitGroup
	ingestClientBatchClient ingestClient
}

type Config struct {
	SendTimeout  time.Duration
	ExportConfig components.Config
}

func (ex *exporter) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.ErrorLevel,
		logrus.FatalLevel,
		logrus.PanicLevel,
		logrus.InfoLevel,
		logrus.WarnLevel,
	}
}

func (ex *exporter) Fire(entry *logrus.Entry) error {
	ex.wg.Add(1)

	go func(entry *logrus.Entry) {
		ex.registrator.WaitUntilRegistered()
		defer ex.wg.Done()
		ex.sendLogEvent(entry)
	}(entry)

	return nil
}

func (ex *exporter) Wait() {
	timeout := 15 * time.Second
	c := make(chan struct{})
	go func() {
		defer close(c)
		ex.wg.Wait()
	}()
	select {
	case <-c:
		return
	case <-time.After(timeout):
		ex.localLog.Error("failed to send logs after shutdown timed out")
		return
	}
}

func (ex *exporter) sendLogEvent(e *logrus.Entry) {
	ctx, cancel := context.WithTimeout(context.Background(), ex.cfg.SendTimeout)
	defer cancel()

	if err := ex.ingestClientBatchClient.IngestLogs(ctx, []components.Entry{
		{
			Level:   e.Level.String(),
			Message: e.Message,
			Time:    e.Time,
			Fields: lo.MapValues(e.Data, func(value any, _ string) string {
				return fmt.Sprintf("%v", value)
			}),
		},
	}); err != nil {
		ex.localLog.Errorf("failed to send logs: %v", err)
	}
}

// InvokeLogrusExitHandlers invokes exit handlers set up in SetupLogExporter
// The handlers are also invoked when any Fatal log entry is made.
// logrus.Exit runs all the Logrus exit handlers and then terminates the program using os.Exit(code)
func InvokeLogrusExitHandlers(err error) {
	if err != nil {
		logrus.Exit(1)
	} else {
		logrus.Exit(0)
	}
}

type ingestClient interface {
	IngestLogs(ctx context.Context, entries []components.Entry) error
	Run(ctx context.Context) error
}
