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

// RunLeaderElection runs leader election and sends status changes to the provided channel.
// The channel is non-blocking - if no receiver is ready, status changes are logged but not blocked.
// Leader election runs continuously until the application context is cancelled.
// If RunOrDie exits unexpectedly, it will automatically restart the leader election process.
func RunLeaderElection(
	ctx context.Context,
	log logrus.FieldLogger,
	cfg config.LeaderElectionConfig,
	client kubernetes.Interface,
	watchDog *leaderelection.HealthzAdaptor,
	leaderStatusCh chan<- bool,
) {
	replicaIdentity := uuid.New().String()
	log = log.WithField("own_identity", replicaIdentity)
	log.Info("starting leader election")

	// Run leader election with automatic restart logic
	// This ensures that if RunOrDie exits for any reason other than context cancellation,
	// the leader election process will restart automatically
	for {
		select {
		case <-ctx.Done():
			log.Info("leader election stopped due to context cancellation")
			return
		default:
		}

		log.Info("initializing leader election attempt")
		runLeaderElection(ctx, log, cfg, client, watchDog, leaderStatusCh, replicaIdentity)

		// RunOrDie has exited - check if it's due to context cancellation (clean shutdown)
		// or an unexpected error (needs restart)
		if ctx.Err() != nil {
			log.Info("leader election stopped due to context cancellation")
			return
		}

		// Context is not cancelled, so RunOrDie exited unexpectedly - restart it
		log.Warn("leader election unexpectedly stopped, restarting in 5 seconds")

		// Wait 5 seconds before restart, but also watch for context cancellation
		// to ensure we can still shut down quickly during the sleep period
		select {
		case <-time.After(5 * time.Second):
			// Continue to restart
		case <-ctx.Done():
			log.Info("leader election stopped due to context cancellation during restart delay")
			return
		}
	}
}

func runLeaderElection(
	ctx context.Context,
	log logrus.FieldLogger,
	cfg config.LeaderElectionConfig,
	client kubernetes.Interface,
	watchDog *leaderelection.HealthzAdaptor,
	leaderStatusCh chan<- bool,
	replicaIdentity string,
) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("leader election panicked: %v", r)
		}
	}()

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
			OnStartedLeading: func(leaderCtx context.Context) {
				log.Info("started leading")
				// Send leadership status change (non-blocking)
				select {
				case leaderStatusCh <- true:
				default:
					log.Warn("leadership status channel full, status change not sent")
				}
			},
			OnStoppedLeading: func() {
				log.Info("stopped leading")
				// Send leadership status change (non-blocking)
				select {
				case leaderStatusCh <- false:
				default:
					log.Warn("leadership status channel full, status change not sent")
				}
			},
			OnNewLeader: func(identity string) {
				if identity == replicaIdentity {
					return
				}
				log.WithField("leader_identity", identity).Info("new leader elected")
			},
		},
	}

	// Run leader election. This runs continuously, automatically handling:
	// - Acquiring leadership when available
	// - Renewing the lease while leader
	// - Releasing leadership and retrying when lease is lost
	// - Only exits when ctx is cancelled (app shutdown) or on panic
	leaderelection.RunOrDie(ctx, leConfig)
}
