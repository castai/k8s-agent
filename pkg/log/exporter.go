package log

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/samber/lo"
	"github.com/sirupsen/logrus"

	"castai-agent/internal/castai"
)

type Exporter interface {
	logrus.Hook
	Wait()
}

func SetupLogExporter(registrator *castai.Registrator, logger *logrus.Logger, localLog logrus.FieldLogger, castaiclient castai.Client, cfg *Config) {
	logExporter := newExporter(registrator, cfg, localLog, castaiclient)
	logger.AddHook(logExporter)
	logrus.RegisterExitHandler(logExporter.Wait)
}

func newExporter(registrator *castai.Registrator, cfg *Config, localLog logrus.FieldLogger, client castai.Client) Exporter {
	return &exporter{
		registrator: registrator,
		cfg:         cfg,
		client:      client,
		localLog:    localLog,
		wg:          sync.WaitGroup{},
	}
}

type exporter struct {
	registrator *castai.Registrator
	localLog    logrus.FieldLogger
	cfg         *Config
	client      castai.Client
	wg          sync.WaitGroup
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
		ex.registrator.WaitUntilRegistered()
		defer ex.wg.Done()
		ex.sendLogEvent(ex.cfg.ClusterID, entry)
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
