package replicas

import (
	"context"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"

	"castai-agent/internal/config"
)

const (
	leaseDuration = 15 * time.Second
	renewDeadline = 10 * time.Second
	retryPeriod   = 2 * time.Second
)

// RunLeaderElection runs leader election and sends status changes to the provided channel.
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

	initialDelay := calculateInitialDelay()
	if initialDelay > 0 {
		log.Infof("waiting %v before attempting leader election", initialDelay)
		select {
		case <-time.After(initialDelay):
			log.Info("initial delay completed, proceeding with leader election")
		case <-ctx.Done():
			log.Info("leader election stopped during initial delay due to context cancellation")
			return
		}
	}

	log.Info("initializing leader election attempt")
	runLeaderElection(ctx, log, cfg, client, watchDog, leaderStatusCh, replicaIdentity)

	if ctx.Err() != nil {
		log.Info("leader election stopped due to context cancellation")
		return
	}

	log.Warn("leader election unexpectedly stopped")

}

func calculateInitialDelay() time.Duration {
	minDelay := 1 * time.Second
	maxDelay := 5 * time.Second
	jitter := time.Duration(rand.Int63n(int64(maxDelay - minDelay)))
	return minDelay + jitter
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
		LeaseDuration:   leaseDuration,
		RenewDeadline:   renewDeadline,
		RetryPeriod:     retryPeriod,
		WatchDog:        watchDog,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(leaderCtx context.Context) {
				log.Info("started leading")
				select {
				case leaderStatusCh <- true:
					log.Debug("successfully sent leadership status: true")
				case <-ctx.Done():
					log.Warn("context cancelled while sending leadership status")
				}
			},
			OnStoppedLeading: func() {
				log.Info("stopped leading")
				select {
				case leaderStatusCh <- false:
					log.Debug("successfully sent leadership status: false")
				case <-ctx.Done():
					log.Warn("context cancelled while sending leadership status")
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
