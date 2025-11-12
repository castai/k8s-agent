package controller

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/metrics/pkg/client/clientset/versioned"

	"castai-agent/pkg/castai"
	"castai-agent/internal/config"
	"castai-agent/internal/services/health"
	"castai-agent/pkg/services/providers/types"
	"castai-agent/internal/services/version"
)

type Params struct {
	Log             logrus.FieldLogger
	Clientset       kubernetes.Interface
	MetricsClient   versioned.Interface
	DynamicClient   dynamic.Interface
	CastaiClient    castai.Client
	Provider        types.Provider
	ClusterID       string
	Config          config.Config
	AgentVersion    *config.AgentVersion
	HealthzProvider *health.HealthzProvider
	LeaderStatusCh  <-chan bool
}

// RunControllerWithRestart runs a controller with restart logic when ctrl.Run() fails.
func RunControllerWithRestart(
	ctx context.Context,
	params *Params,
) error {
	defer func() {
		params.Log.Info("stopping controller")
		if err := recover(); err != nil {
			params.Log.Errorf("controller panic: %v", err)
		}
	}()

	return repeatUntilContextClosed(ctx, func(ctx context.Context) error {
		log := params.Log.WithField("controller_id", uuid.New().String())

		v, err := version.Get(log, params.Clientset)
		if err != nil {
			return fmt.Errorf("getting kubernetes version: %w", err)
		}

		log = log.WithField("k8s_version", v.Full())

		ctrl := New(
			params.Log,
			params.Clientset,
			params.DynamicClient,
			params.CastaiClient,
			params.MetricsClient,
			params.Provider,
			params.ClusterID,
			params.Config.Controller,
			v,
			params.AgentVersion,
			params.HealthzProvider,
			params.Clientset.AuthorizationV1().SelfSubjectAccessReviews(),
			params.Config.SelfPod.Namespace,
			params.LeaderStatusCh,
		)

		// Start informers
		ctrl.Start(ctx.Done())

		if err := ctrl.Run(ctx); err != nil {
			params.Log.Errorf("controller run error, will recreate and restart: %v", err)
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
