package controller

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/metrics/pkg/client/clientset/versioned"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/providers/types"
	"castai-agent/internal/services/version"
)

func Loop(
	ctx context.Context,
	log logrus.FieldLogger,
	clientset kubernetes.Interface,
	metricsClient versioned.Interface,
	dynamicClient dynamic.Interface,
	castaiclient castai.Client,
	provider types.Provider,
	clusterID string,
	cfg config.Config,
	agentVersion *config.AgentVersion,
	healthzProvider *HealthzProvider,
) error {
	return repeatUntilContextClosed(ctx, func(ctx context.Context) error {
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

		ctrl := New(
			log,
			clientset,
			dynamicClient,
			castaiclient,
			metricsClient,
			provider,
			clusterID,
			cfg.Controller,
			v,
			agentVersion,
			healthzProvider,
			clientset.AuthorizationV1().SelfSubjectAccessReviews(),
			cfg.SelfPod.Namespace,
		)

		ctrl.Start(ctrlCtx.Done())

		// Loop the controller. This is a blocking call.
		return ctrl.Run(ctrlCtx)
	})
}

func repeatUntilContextClosed(ctx context.Context, fn func(context.Context) error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := fn(ctx); err != nil {
			return fmt.Errorf("running controller function: %w", err)
		}
	}
}
