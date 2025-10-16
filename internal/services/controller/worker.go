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

// ControllerFactory contains all dependencies needed to create a controller
type ControllerFactory struct {
	Log             logrus.FieldLogger
	Clientset       kubernetes.Interface
	MetricsClient   versioned.Interface
	DynamicClient   dynamic.Interface
	CastaiClient    castai.Client
	Provider        types.Provider
	ClusterID       string
	Config          config.Config
	AgentVersion    *config.AgentVersion
	HealthzProvider *HealthzProvider
	LeaderStatusCh  <-chan bool
}

// RunControllerWithRestart runs a controller with restart logic when ctrl.Run() fails.
// This properly recreates the controller on each restart to ensure all initialization logic runs.
func RunControllerWithRestart(
	ctx context.Context,
	factory *ControllerFactory,
) error {
	defer func() {
		factory.Log.Info("stopping controller")
		if err := recover(); err != nil {
			factory.Log.Errorf("controller panic: %v", err)
		}
	}()

	return repeatUntilContextClosed(ctx, func(ctx context.Context) error {
		log := factory.Log.WithField("controller_id", uuid.New().String())

		v, err := version.Get(log, factory.Clientset)
		if err != nil {
			return fmt.Errorf("getting kubernetes version: %w", err)
		}

		log = log.WithField("k8s_version", v.Full())

		ctrl := New(
			factory.Log,
			factory.Clientset,
			factory.DynamicClient,
			factory.CastaiClient,
			factory.MetricsClient,
			factory.Provider,
			factory.ClusterID,
			factory.Config.Controller,
			v,
			factory.AgentVersion,
			factory.HealthzProvider,
			factory.Clientset.AuthorizationV1().SelfSubjectAccessReviews(),
			factory.Config.SelfPod.Namespace,
			factory.LeaderStatusCh,
		)

		// Start informers
		ctrl.Start(ctx.Done())

		// Run the controller
		if err := ctrl.Run(ctx); err != nil {
			factory.Log.Errorf("controller run error, will recreate and restart: %v", err)
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

		logrus.Info("controller iteration starting")
		if err := fn(ctx); err != nil {
			return fmt.Errorf("running controller function: %w", err)
		}
	}
}
