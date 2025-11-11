package replicas

import (
	"context"
	"fmt"
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
// Returns an error if leader election stops unexpectedly (not due to context cancellation).
func RunLeaderElection(
	ctx context.Context,
	log logrus.FieldLogger,
	cfg config.LeaderElectionConfig,
	client kubernetes.Interface,
	watchDog *leaderelection.HealthzAdaptor,
	leaderStatusCh chan<- bool,
) error {
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
			return nil
		}
	}

	log.Info("initializing leader election attempt")

	// Start watchdog to regularly send leader status updates
	// Use a separate context so watchdog stops when runLeaderElection exits
	watchdogCtx, watchdogCancel := context.WithCancel(ctx)
	defer watchdogCancel()

	go runLeaseWatchdog(watchdogCtx, log, cfg, client, leaderStatusCh, replicaIdentity)

	runLeaderElection(ctx, log, cfg, client, watchDog, leaderStatusCh, replicaIdentity)

	// Always send false to leadership channel when exiting
	// This ensures controller knows we're not leading anymore
	select {
	case leaderStatusCh <- false:
		log.Info("sent final leadership status: false")
	case <-time.After(2 * time.Second):
		log.Warn("timeout sending final leadership status")
	}

	if ctx.Err() != nil {
		log.Info("leader election stopped due to context cancellation")
		return nil
	}

	log.Error("leader election stopped unexpectedly - this should trigger app shutdown")
	return fmt.Errorf("leader election stopped unexpectedly")
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
	// Capture panics to ensure proper cleanup and logging
	// Then re-panic to trigger RunOrDie's error handling and lease release
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("leader election panicked: %v", r)
			// Re-panic to let RunOrDie handle it and release the lease
			panic(r)
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
					log.Info("successfully sent leadership status: true")
				case <-ctx.Done():
					log.Warn("context cancelled while sending leadership status")
				}
			},
			OnStoppedLeading: func() {
				log.Info("stopped leading")
				select {
				case leaderStatusCh <- false:
					log.Info("successfully sent leadership status: false")
				case <-ctx.Done():
					log.Warn("context cancelled while sending leadership status")
				}
			},
			OnNewLeader: func(identity string) {
				if identity == replicaIdentity {
					return
				}
				select {
				case leaderStatusCh <- false:
					log.Info("successfully sent leadership status: false (new leader detected)")
				case <-ctx.Done():
					log.Warn("context cancelled while sending leadership status")
				}
				log.WithField("leader_identity", identity).Info("current leader")
			},
		},
	}

	// Run leader election. This runs continuously, automatically handling:
	// - Acquiring leadership when available
	// - Renewing the lease while leader
	// - Exits when leadership is lost or context is cancelled
	// Allow the function to panic or exit if context is cancelled.
	// This ensures the lease is properly released via ReleaseOnCancel: true
	// and the app context is cancelled to allow another replica to take over
	leaderelection.RunOrDie(ctx, leConfig)
}

// queryCurrentLeaseHolder queries the Kubernetes API to check who currently holds the lease.
// Returns empty string if no one holds the lease or if there's an error querying.
func queryCurrentLeaseHolder(
	ctx context.Context,
	log logrus.FieldLogger,
	cfg config.LeaderElectionConfig,
	client kubernetes.Interface,
) string {
	lease, err := client.CoordinationV1().Leases(cfg.Namespace).Get(ctx, cfg.LockName, metav1.GetOptions{})
	if err != nil {
		log.WithError(err).Debug("failed to query current lease holder (may not exist yet)")
		return ""
	}

	if lease.Spec.HolderIdentity == nil {
		log.Debug("lease exists but has no holder")
		return ""
	}

	holderIdentity := *lease.Spec.HolderIdentity

	// Check if lease is expired
	if lease.Spec.RenewTime != nil {
		var duration time.Duration
		if lease.Spec.LeaseDurationSeconds != nil {
			duration = time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second
		} else {
			duration = leaseDuration // your default
		}
		leaseExpiry := lease.Spec.RenewTime.Add(duration)
		if time.Now().After(leaseExpiry) {
			log.WithFields(logrus.Fields{
				"holder":     holderIdentity,
				"renew_time": lease.Spec.RenewTime.Time,
				"expiry":     leaseExpiry,
			}).Debug("lease exists but is expired")
			return ""
		}
	}

	log.WithField("holder", holderIdentity).Debug("found current lease holder")
	return holderIdentity
}

// runLeaseWatchdog periodically verifies the lease state and sends regular updates to leader chanel
func runLeaseWatchdog(
	ctx context.Context,
	log logrus.FieldLogger,
	cfg config.LeaderElectionConfig,
	client kubernetes.Interface,
	leaderStatusCh chan<- bool,
	replicaIdentity string,
) {
	ticker := time.NewTicker(leaseDuration)
	defer ticker.Stop()

	log.Debug("starting lease watchdog")

	for {
		select {
		case <-ctx.Done():
			log.Debug("lease watchdog stopped due to context cancellation")
			return
		case <-ticker.C:
			currentHolder := queryCurrentLeaseHolder(ctx, log, cfg, client)

			isLeader := false
			if currentHolder == "" {
				log.WithField("own_identity", replicaIdentity).Debug("watchdog: no current lease holder")
			} else if currentHolder == replicaIdentity {
				log.WithField("own_identity", replicaIdentity).Debug("watchdog: this replica holds the lease")
				isLeader = true
			} else {
				log.WithFields(logrus.Fields{
					"current_holder": currentHolder,
					"own_identity":   replicaIdentity,
				}).Debug("watchdog: another replica holds the lease")
			}
			leaderStatusCh <- isLeader
		}
	}
}
