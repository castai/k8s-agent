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
)

type Replica func(ctx context.Context)

// Run will block waiting until this replica becomes the leader,
// then runs replica until it stops or is no longer a leader
func Run(
	ctx context.Context,
	log logrus.FieldLogger,
	cfg config.LeaderElectionConfig,
	client kubernetes.Interface,
	watchDog *leaderelection.HealthzAdaptor,
	replica Replica,
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
				replica(ctx)
			},
			OnStoppedLeading: func() {
				log.Info("stopped leading")
			},
		},
	}

	leaderelection.RunOrDie(ctx, leConfig)
}
