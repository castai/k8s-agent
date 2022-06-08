package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/providers/types"
	"castai-agent/internal/services/version"
)

type Worker struct {
	Fn func(ctx context.Context) error

	stop   context.CancelFunc
	exitCh chan struct{}
}

func (w *Worker) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	w.exitCh = make(chan struct{})
	defer close(w.exitCh)

	w.stop = cancel

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := w.Fn(ctx); err != nil {
			return fmt.Errorf("running controller function: %w", err)
		}
	}
}

func (w *Worker) Stop(log logrus.FieldLogger) {
	if w.stop == nil {
		return
	}

	w.stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	select {
	case <-w.exitCh:
	case <-ctx.Done():
		log.Errorf("waiting for controller to exit: %v", ctx.Err())
	}
}

func RunController(
	log logrus.FieldLogger,
	clientset kubernetes.Interface,
	castaiclient castai.Client,
	provider types.Provider,
	clusterID string,
	cfg config.Config,
	agentVersion *config.AgentVersion,
	healthzProvider *HealthzProvider,
) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		log = log.WithField("controller_id", uuid.New().String())

		defer func() {
			if err := recover(); err != nil {
				log.Errorf("panic: runtime error: %v", err)
			}
		}()

		ctrlCtx, cancelCtrlCtx := context.WithCancel(ctx)
		defer cancelCtrlCtx()

		v, err := version.Get(log, clientset)
		if err != nil {
			return fmt.Errorf("getting kubernetes version: %w", err)
		}

		log = log.WithField("k8s_version", v.Full())

		f := informers.NewSharedInformerFactory(clientset, 0)
		ctrl := New(
			log,
			f,
			castaiclient,
			provider,
			clusterID,
			cfg.Controller,
			v,
			agentVersion,
			healthzProvider,
		)
		f.Start(ctrlCtx.Done())

		// Run the controller. This is a blocking call.
		return ctrl.Run(ctrlCtx)
	}
}
