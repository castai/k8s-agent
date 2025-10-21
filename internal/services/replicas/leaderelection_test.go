package replicas

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
					assert.True(t, isLeader, "should become leader")
				case <-time.After(6 * time.Second):
					t.Fatal("timeout waiting for leadership")
				}

				ctx := context.Background()
				leases, err := clientset.CoordinationV1().Leases(cfg.Namespace).List(ctx, metav1.ListOptions{})
				require.NoError(t, err)
				assert.Len(t, leases.Items, 1, "should have one lease")
				assert.Equal(t, cfg.LockName, leases.Items[0].Name)
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

			ctx, cancel := context.WithTimeout(context.Background(), tt.contextTimeout)
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
