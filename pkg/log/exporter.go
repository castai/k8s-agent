package log

import (
	"context"
	"sync"
	"time"

	"castai-agent/internal/castai"

	"github.com/sirupsen/logrus"
)

func NewExporter(cfg Config, log logrus.Logger, client castai.Client) logrus.Hook {
	return &exporter{
		cfg:    cfg,
		log:    log,
		client: client,
	}
}

type exporter struct {
	cfg    Config
	log    logrus.Logger
	client castai.Client
	wg     sync.WaitGroup
}

type Config struct {
	MsgSendTimeout time.Duration
}

type LogEvent struct {
	Level   string        `json:"level"`
	Time    time.Time     `json:"time"`
	Message string        `json:"message"`
	Fields  logrus.Fields `json:"fields"`
}

func (ex *exporter) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.ErrorLevel,
		logrus.FatalLevel,
		logrus.PanicLevel,
		logrus.WarnLevel,
	}
}

func (ex *exporter) Fire(entry *logrus.Entry) error {
	ex.wg.Add(1)
	go func(entry *logrus.Entry) {
		defer ex.wg.Done()

	}(entry)
	return nil
}

func (ex *exporter) sendLogEvent(e *LogEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), ex.cfg.MsgSendTimeout)
	defer cancel()

	return ex.client.SendDelta(ctx, nil)
}
