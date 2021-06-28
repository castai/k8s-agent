package log

import (
	"context"
	"sync"
	"time"

	"castai-agent/internal/castai"

	"github.com/sirupsen/logrus"
)

type Exporter interface {
	logrus.Hook
	Wait()
}

func SetupLogExporter(logger *logrus.Logger, clusterID string, castaiclient castai.Client) {
	logExporter := newExporter(Config{
		ClusterID:          clusterID,
		MsgSendTimeoutSecs: 15000,
	}, castaiclient)

	logger.AddHook(logExporter)
	logrus.RegisterExitHandler(logExporter.Wait)
}

func newExporter(cfg Config, client castai.Client) Exporter {
	return &exporter{
		cfg:    cfg,
		client: client,
		wg:     sync.WaitGroup{},
	}
}

type exporter struct {
	cfg    Config
	client castai.Client
	wg     sync.WaitGroup
}

type Config struct {
	ClusterID          string
	MsgSendTimeoutSecs time.Duration
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

// Wait will return after all subroutines have returned.
// Use in conjunction with logrus return handling to ensure all of
// your logs are delivered before your program exits.
// `logrus.RegisterExitHandler(h.Wait)`
func (ex *exporter) Wait() {
	ex.wg.Wait()
}

func (ex *exporter) sendLogEvent(clusterID string, e *logrus.Entry) {
	//ctx, cancel := context.WithTimeout(context.Background(), ex.cfg.MsgSendTimeoutSecs)
	//defer cancel()
	ctx := context.Background()

	ex.client.SendLogEvent(
		ctx,
		clusterID,
		&castai.SendLogEventRequest{
			LogEvent: castai.LogEvent{
				Level:   e.Level.String(),
				Time:    e.Time,
				Message: e.Message,
				Fields:  e.Data,
			},
		})
}
