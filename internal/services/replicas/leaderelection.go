package replicas

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"

	"castai-agent/internal/config"
	"castai-agent/internal/services/controller"
)

// RunWithSharedController runs leader election with a pre-created controller instance
// that continues running regardless of leadership changes
func RunWithSharedController(
	ctx context.Context,
	log logrus.FieldLogger,
	cfg config.LeaderElectionConfig,
	client kubernetes.Interface,
	watchDog *leaderelection.HealthzAdaptor,
	controller *controller.Controller,
) {
	replicaIdentity := uuid.New().String()
	log = log.WithField("own_identity", replicaIdentity)
	log.Info("starting with leader election")

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      cfg.LockName,
			Namespace: cfg.Namespace,
		},
		Client: client.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: replicaIdentity,
		},
	}

	leConfig := leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   15 * time.Second,
		RenewDeadline:   10 * time.Second,
		RetryPeriod:     2 * time.Second,
		WatchDog:        watchDog,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				log.Info("started leading")
				controller.SetLeader(true)
			},
			OnStoppedLeading: func() {
				log.Info("stopped leading")
				controller.SetLeader(false)
			},
			OnNewLeader: func(identity string) {
				if identity == replicaIdentity {
					return
				}
				log.WithField("leader_identity", identity).Info("new leader elected")
			},
		},
	}

	leaderelection.RunOrDie(ctx, leConfig)
}

// Legacy Run function for backward compatibility
func Run(
	ctx context.Context,
	log logrus.FieldLogger,
	cfg config.LeaderElectionConfig,
	client kubernetes.Interface,
	watchDog *leaderelection.HealthzAdaptor,
	runController func(ctx context.Context, ctrl *controller.Controller),
) {
	replicaIdentity := uuid.New().String()
	log = log.WithField("own_identity", replicaIdentity)
	log.Info("starting with leader election")

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      cfg.LockName,
			Namespace: cfg.Namespace,
		},
		Client: client.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: replicaIdentity,
		},
	}

	leConfig := leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   15 * time.Second,
		RenewDeadline:   10 * time.Second,
		RetryPeriod:     2 * time.Second,
		WatchDog:        watchDog,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				log.Info("started leading")
				runController(ctx, nil)
			},
			OnStoppedLeading: func() {
				log.Info("stopped leading")
			},
		},
	}

	leaderelection.RunOrDie(ctx, leConfig)
}
