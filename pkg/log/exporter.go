package log

import (
	"context"
	"fmt"
	"time"

	"github.com/samber/lo"
	"golang.org/x/sync/semaphore"

	"castai-agent/internal/castai"

	"github.com/sirupsen/logrus"
)

const MaxParallelLogs int64 = 100

func SetupLogExporter(registrator *castai.Registrator, logger *logrus.Logger, localLog logrus.FieldLogger, castaiclient castai.Client, cfg *Config) *Exporter {
	logExporter := &Exporter{
		registrator: registrator,
		cfg:         cfg,
		client:      castaiclient,
		localLog:    localLog,
		sem:         semaphore.NewWeighted(MaxParallelLogs),
	}
	logger.AddHook(logExporter)

	return logExporter
}

type Exporter struct {
	registrator *castai.Registrator
	localLog    logrus.FieldLogger
	cfg         *Config
	client      castai.Client
	sem         *semaphore.Weighted
}

type Config struct {
	ClusterID    string
	SendTimeout  time.Duration
	FlushTimeout time.Duration
}

func (ex *Exporter) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.ErrorLevel,
		logrus.FatalLevel,
		logrus.PanicLevel,
		logrus.InfoLevel,
		logrus.WarnLevel,
	}
}

func (ex *Exporter) Fire(entry *logrus.Entry) error {
	if err := ex.sem.Acquire(context.Background(), 1); err != nil {
		return err
	}

	go func(entry *logrus.Entry) {
		defer ex.sem.Release(1)

		ex.registrator.WaitUntilRegistered()
		ex.sendLogEvent(ex.cfg.ClusterID, entry)
	}(entry)

	return nil
}

func (ex *Exporter) Wait() {
	ctx, cancel := context.WithTimeout(context.Background(), ex.cfg.FlushTimeout)
	defer cancel()

	err := ex.sem.Acquire(ctx, MaxParallelLogs)
	if err != nil {
		ex.localLog.Errorf("failed to flush logs: %v", err)
	}
}

func (ex *Exporter) sendLogEvent(clusterID string, e *logrus.Entry) {
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
