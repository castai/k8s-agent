package replicas

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"

	"castai-agent/internal/config"
	"castai-agent/pkg/services/providers/types"
)

const (
	registrationLeaseDuration = 5 * time.Second
	registrationRenewDeadline = 4 * time.Second
	registrationRetryPeriod   = 1 * time.Second
)

// RegisterClusterWithLease serializes cluster registration across replicas using a short-lived
// Kubernetes lease. Each pod acquires the lease, calls registerFn, then releases the lease so
// the next pod can proceed.
func RegisterClusterWithLease(
	ctx context.Context,
	log logrus.FieldLogger,
	cfg config.LeaderElectionConfig,
	client kubernetes.Interface,
	registerFn func(ctx context.Context) (*types.ClusterRegistration, error),
) (*types.ClusterRegistration, error) {
	leaseName := cfg.LockName + "-registration"
	identity := uuid.New().String()

	log = log.WithFields(logrus.Fields{
		"registration_identity": identity,
		"registration_lease":    leaseName,
	})
	log.Info("acquiring registration lease")

	var result *types.ClusterRegistration
	var registerErr error

	// leaderCtx is cancelled by leaderCancel once registration completes,
	// causing the leader elector to release the lease and Run() to return.
	leaderCtx, leaderCancel := context.WithCancel(ctx)
	defer leaderCancel()

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      leaseName,
			Namespace: cfg.Namespace,
		},
		Client: client.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: identity,
		},
	}

	le, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   registrationLeaseDuration,
		RenewDeadline:   registrationRenewDeadline,
		RetryPeriod:     registrationRetryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(innerCtx context.Context) {
				log.Info("registration lease acquired, registering cluster")
				result, registerErr = registerFn(innerCtx)
				leaderCancel()
			},
			OnStoppedLeading: func() {
				log.Info("registration lease released")
			},
			OnNewLeader: func(identity string) {},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating registration leader elector: %w", err)
	}

	le.Run(leaderCtx)

	if registerErr != nil {
		return nil, registerErr
	}
	if result == nil {
		// Run returned without calling OnStartedLeading — context was canceled before acquisition
		return nil, ctx.Err()
	}

	log.Info("registration lease complete")
	return result, nil
}
