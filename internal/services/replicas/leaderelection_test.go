package replicas

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/leaderelection"

	"castai-agent/internal/config"
)

func TestRunLeaderElection(t *testing.T) {
	tests := map[string]struct {
		setupClientset    func() kubernetes.Interface
		contextTimeout    time.Duration
		cancelImmediately bool
		verifyFn          func(t *testing.T, leaderStatusCh chan bool, clientset kubernetes.Interface, cfg config.LeaderElectionConfig)
	}{
		"becomes leader successfully": {
			setupClientset: func() kubernetes.Interface {
				return fake.NewSimpleClientset()
			},
			contextTimeout: 10 * time.Second,
			verifyFn: func(t *testing.T, leaderStatusCh chan bool, clientset kubernetes.Interface, cfg config.LeaderElectionConfig) {
				select {
				case isLeader := <-leaderStatusCh:
					require.True(t, isLeader, "should become leader")
				case <-time.After(6 * time.Second):
					t.Fatal("timeout waiting for leadership")
				}

				leases, err := clientset.CoordinationV1().Leases(cfg.Namespace).List(t.Context(), metav1.ListOptions{})
				require.NoError(t, err)
				require.Len(t, leases.Items, 1, "should have one lease")
				require.Equal(t, cfg.LockName, leases.Items[0].Name)
			},
		},
		"handles context cancellation during initial delay": {
			setupClientset: func() kubernetes.Interface {
				return fake.NewSimpleClientset()
			},
			contextTimeout:    2 * time.Second,
			cancelImmediately: true,
			verifyFn: func(t *testing.T, leaderStatusCh chan bool, clientset kubernetes.Interface, cfg config.LeaderElectionConfig) {
				select {
				case <-leaderStatusCh:
				case <-time.After(100 * time.Millisecond):
				}
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {

			ctx, cancel := context.WithTimeout(t.Context(), tt.contextTimeout)
			defer cancel()

			log := logrus.New()
			log.SetLevel(logrus.DebugLevel)

			cfg := config.LeaderElectionConfig{
				LockName:  "test-lock",
				Namespace: "default",
			}

			clientset := tt.setupClientset()
			watchDog := leaderelection.NewLeaderHealthzAdaptor(time.Second)

			leaderStatusCh := make(chan bool, 10)

			if tt.cancelImmediately {
				cancel()
			}

			// Run leader election in a goroutine
			done := make(chan struct{})
			go func() {
				defer close(done)
				RunLeaderElection(ctx, log, cfg, clientset, watchDog, leaderStatusCh)
			}()

			// Run verification
			if tt.verifyFn != nil {
				tt.verifyFn(t, leaderStatusCh, clientset, cfg)
			}

			// Cleanup
			cancel()
			select {
			case <-done:
				// Success
			case <-time.After(3 * time.Second):
				t.Fatal("timeout waiting for leader election to stop")
			}
		})
	}
}

func TestQueryCurrentLeaseHolder(t *testing.T) {
	tests := map[string]struct {
		setupLease     func(clientset *fake.Clientset, cfg config.LeaderElectionConfig)
		expectedHolder string
	}{
		"returns holder when valid lease exists": {
			setupLease: func(clientset *fake.Clientset, cfg config.LeaderElectionConfig) {
				holder := "replica-123"
				renewTime := metav1.NewMicroTime(time.Now())
				lease := &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cfg.LockName,
						Namespace: cfg.Namespace,
					},
					Spec: coordinationv1.LeaseSpec{
						HolderIdentity: &holder,
						RenewTime:      &renewTime,
					},
				}
				_, _ = clientset.CoordinationV1().Leases(cfg.Namespace).Create(t.Context(), lease, metav1.CreateOptions{})
			},
			expectedHolder: "replica-123",
		},
		"returns empty when lease does not exist": {
			setupLease:     func(clientset *fake.Clientset, cfg config.LeaderElectionConfig) {},
			expectedHolder: "",
		},
		"returns empty when lease has no holder": {
			setupLease: func(clientset *fake.Clientset, cfg config.LeaderElectionConfig) {
				lease := &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cfg.LockName,
						Namespace: cfg.Namespace,
					},
					Spec: coordinationv1.LeaseSpec{
						HolderIdentity: nil,
					},
				}
				_, _ = clientset.CoordinationV1().Leases(cfg.Namespace).Create(t.Context(), lease, metav1.CreateOptions{})
			},
			expectedHolder: "",
		},
		"returns empty when lease is expired": {
			setupLease: func(clientset *fake.Clientset, cfg config.LeaderElectionConfig) {
				holder := "replica-456"
				expiredTime := metav1.NewMicroTime(time.Now().Add(-20 * time.Second))
				lease := &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cfg.LockName,
						Namespace: cfg.Namespace,
					},
					Spec: coordinationv1.LeaseSpec{
						HolderIdentity: &holder,
						RenewTime:      &expiredTime,
					},
				}
				_, _ = clientset.CoordinationV1().Leases(cfg.Namespace).Create(t.Context(), lease, metav1.CreateOptions{})
			},
			expectedHolder: "",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			log := logrus.New()
			log.SetLevel(logrus.DebugLevel)

			cfg := config.LeaderElectionConfig{
				LockName:  "test-lock",
				Namespace: "default",
			}

			clientset := fake.NewSimpleClientset()
			tt.setupLease(clientset, cfg)

			holder := queryCurrentLeaseHolder(ctx, log, cfg, clientset)
			require.Equal(t, tt.expectedHolder, holder)
		})
	}
}

func TestRunLeaseWatchdog(t *testing.T) {
	leaseDurationSeconds := lo.ToPtr(int32(leaseDuration.Seconds()) + 1)
	tests := map[string]struct {
		replicaIdentity string
		setupLease      func(clientset *fake.Clientset, cfg config.LeaderElectionConfig)
		expectedLeader  bool
		description     string
	}{
		"sends true when this replica holds lease": {
			replicaIdentity: "replica-123",
			setupLease: func(clientset *fake.Clientset, cfg config.LeaderElectionConfig) {
				holder := "replica-123"
				renewTime := metav1.NewMicroTime(time.Now())
				lease := &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cfg.LockName,
						Namespace: cfg.Namespace,
					},
					Spec: coordinationv1.LeaseSpec{
						LeaseDurationSeconds: leaseDurationSeconds,
						HolderIdentity:       &holder,
						RenewTime:            &renewTime,
					},
				}
				_, _ = clientset.CoordinationV1().Leases(cfg.Namespace).Create(t.Context(), lease, metav1.CreateOptions{})
			},
			expectedLeader: true,
		},
		"sends false when another replica holds lease": {
			replicaIdentity: "replica-123",
			setupLease: func(clientset *fake.Clientset, cfg config.LeaderElectionConfig) {
				holder := "replica-456"
				renewTime := metav1.NewMicroTime(time.Now())
				lease := &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cfg.LockName,
						Namespace: cfg.Namespace,
					},
					Spec: coordinationv1.LeaseSpec{
						LeaseDurationSeconds: leaseDurationSeconds,
						HolderIdentity:       &holder,
						RenewTime:            &renewTime,
					},
				}
				_, _ = clientset.CoordinationV1().Leases(cfg.Namespace).Create(t.Context(), lease, metav1.CreateOptions{})
			},
			expectedLeader: false,
		},
		"sends false when no lease exists": {
			replicaIdentity: "replica-123",
			setupLease:      func(clientset *fake.Clientset, cfg config.LeaderElectionConfig) {},
			expectedLeader:  false,
		},
		"sends false when lease is expired": {
			replicaIdentity: "replica-123",
			setupLease: func(clientset *fake.Clientset, cfg config.LeaderElectionConfig) {
				holder := "replica-123"
				expiredTime := metav1.NewMicroTime(time.Now().Add(-(leaseDuration + 2))) // make sure the lease is expired
				lease := &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cfg.LockName,
						Namespace: cfg.Namespace,
					},
					Spec: coordinationv1.LeaseSpec{
						LeaseDurationSeconds: leaseDurationSeconds,
						HolderIdentity:       &holder,
						RenewTime:            &expiredTime,
					},
				}
				_, _ = clientset.CoordinationV1().Leases(cfg.Namespace).Create(t.Context(), lease, metav1.CreateOptions{})
			},
			expectedLeader: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			log := logrus.New()
			log.SetLevel(logrus.DebugLevel)

			cfg := config.LeaderElectionConfig{
				LockName:  "test-lock",
				Namespace: "default",
			}

			clientset := fake.NewClientset()
			tt.setupLease(clientset, cfg)

			leaderStatusCh := make(chan bool, 10)

			go runLeaseWatchdog(t.Context(), log, cfg, clientset, leaderStatusCh, tt.replicaIdentity)

			select {
			case isLeader := <-leaderStatusCh:
				require.Equal(t, tt.expectedLeader, isLeader)
			case <-time.After(leaseDuration + 2*time.Second):
				t.Fatal("timeout waiting for watchdog status")
			}
		})
	}
}
