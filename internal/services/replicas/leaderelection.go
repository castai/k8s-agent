package replicas

import (
	"castai-agent/internal/config"
	"context"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"time"
)

type Replica interface {
	RunAsLeader(ctx context.Context)
	Stop()
}

func startWithLeaderElection(
	ctx context.Context,
	log logrus.FieldLogger,
	replica Replica,
	cfg config.LeaderElectionConfig,
	client kubernetes.Interface,
	check *leaderelection.HealthzAdaptor,
) error {
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

	for {
		// attempt to run as leader once
		leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
			Lock:            lock,
			ReleaseOnCancel: true,
			LeaseDuration:   15 * time.Second,
			RenewDeadline:   10 * time.Second,
			RetryPeriod:     2 * time.Second,
			WatchDog:        check,
			Callbacks: leaderelection.LeaderCallbacks{
				OnStartedLeading: func(ctx context.Context) {
					log.Info("started leading")
					replica.RunAsLeader(ctx)
				},
				OnStoppedLeading: func() {
					log.Info("stopped leading")
					replica.Stop()
				},
			},
		})

		// retry leader lock until ctx canceled
		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}

	}
}
