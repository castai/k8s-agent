package log

import (
	"context"
	"fmt"
	"github.com/samber/lo"
	"sync"
	"time"

	"castai-agent/internal/castai"

	"github.com/sirupsen/logrus"
)

type Exporter interface {
	logrus.Hook
	Wait()
}

func SetupLogExporter(logger *logrus.Logger, localLog logrus.FieldLogger, castaiclient castai.Client, cfg *Config) {
	logExporter := newExporter(cfg, localLog, castaiclient)
	logger.AddHook(logExporter)
	logrus.RegisterExitHandler(logExporter.Wait)
}

func newExporter(cfg *Config, localLog logrus.FieldLogger, client castai.Client) Exporter {
	return &exporter{
		cfg:      cfg,
		client:   client,
		localLog: localLog,
		wg:       sync.WaitGroup{},
	}
}

type exporter struct {
	localLog logrus.FieldLogger
	cfg      *Config
	client   castai.Client
	wg       sync.WaitGroup
}

type Config struct {
	ClusterID   string
	SendTimeout time.Duration
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
		defer ex.wg.Done()
		ex.sendLogEvent(ex.cfg.ClusterID, entry)
	}(entry)

	return nil
}

func (ex *exporter) Wait() {
	ex.wg.Wait()
}

func (ex *exporter) sendLogEvent(clusterID string, e *logrus.Entry) {
	ctx, cancel := context.WithTimeout(context.Background(), ex.cfg.SendTimeout)
	defer cancel()

	_, err := ex.client.SendLogEvent(
		ctx,
		clusterID,
		&castai.IngestAgentLogsRequest{
			LogEvent: castai.LogEvent{
				Level:   e.Level.String(),
				Time:    e.Time,
				Message: e.Message,
				Fields: lo.MapValues(e.Data, func(value interface{}, _ string) string {
					return fmt.Sprintf("%v", value)
				}),
			},
		})
	if err != nil {
		ex.localLog.Errorf("failed to send logs: %v", err)
	}
}
