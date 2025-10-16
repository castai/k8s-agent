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

// CreateController creates a new controller instance without the restart loop.
// This is used for the continuous informer pattern where the controller runs continuously
// and only leadership status controls data sending.
func CreateController(
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
) (*Controller, error) {
	log = log.WithField("controller_id", uuid.New().String())

	v, err := version.Get(log, clientset)
	if err != nil {
		return nil, fmt.Errorf("getting kubernetes version: %w", err)
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

	return ctrl, nil
}

// RunControllerWithRestart runs a controller with restart logic when ctrl.Run() fails.
// This is used for the continuous informer pattern where informers stay running
// but the controller logic can restart on errors.
func RunControllerWithRestart(ctx context.Context, log logrus.FieldLogger, ctrl *Controller) error {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("controller panic: %v", err)
		}
	}()

	// Use the same restart pattern as repeatUntilContextClosed
	return repeatUntilContextClosed(ctx, func(ctx context.Context) error {
		if err := ctrl.Run(ctx); err != nil {
			log.Errorf("controller run error, restarting: %w", err)
			return err
		}
		return nil
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
