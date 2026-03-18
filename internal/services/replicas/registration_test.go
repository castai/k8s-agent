package replicas

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"castai-agent/internal/config"
	"castai-agent/pkg/services/providers/types"
)

func TestRegisterClusterWithLease(t *testing.T) {
	tests := map[string]struct {
		registerFn func(ctx context.Context) (*types.ClusterRegistration, error)
		setupCtx   func() (context.Context, context.CancelFunc)
		verify     func(t *testing.T, reg *types.ClusterRegistration, err error, clientset *fake.Clientset, cfg config.LeaderElectionConfig)
	}{
		"registers cluster successfully": {
			registerFn: func(ctx context.Context) (*types.ClusterRegistration, error) {
				return &types.ClusterRegistration{
					ClusterID:      "cluster-123",
					OrganizationID: "org-456",
				}, nil
			},
			setupCtx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 10*time.Second)
			},
			verify: func(t *testing.T, reg *types.ClusterRegistration, err error, clientset *fake.Clientset, cfg config.LeaderElectionConfig) {
				require.NoError(t, err)
				require.NotNil(t, reg)
				require.Equal(t, "cluster-123", reg.ClusterID)
				require.Equal(t, "org-456", reg.OrganizationID)

				// Verify lease was created
				leaseName := cfg.LockName + "-registration"
				lease, err := clientset.CoordinationV1().Leases(cfg.Namespace).Get(context.Background(), leaseName, metav1.GetOptions{})
				require.NoError(t, err)
				require.NotNil(t, lease)
			},
		},
		"propagates registration error": {
			registerFn: func(ctx context.Context) (*types.ClusterRegistration, error) {
				return nil, fmt.Errorf("api unavailable")
			},
			setupCtx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 10*time.Second)
			},
			verify: func(t *testing.T, reg *types.ClusterRegistration, err error, clientset *fake.Clientset, cfg config.LeaderElectionConfig) {
				require.Error(t, err)
				require.ErrorContains(t, err, "api unavailable")
				require.Nil(t, reg)
			},
		},
		"returns error on context cancellation": {
			registerFn: func(ctx context.Context) (*types.ClusterRegistration, error) {
				return &types.ClusterRegistration{ClusterID: "should-not-reach"}, nil
			},
			setupCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // cancel immediately
				return ctx, cancel
			},
			verify: func(t *testing.T, reg *types.ClusterRegistration, err error, clientset *fake.Clientset, cfg config.LeaderElectionConfig) {
				require.Error(t, err)
				require.ErrorIs(t, err, context.Canceled)
				require.Nil(t, reg)
			},
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

			ctx, cancel := tt.setupCtx()
			defer cancel()

			reg, err := RegisterClusterWithLease(ctx, log, cfg, clientset, tt.registerFn)
			tt.verify(t, reg, err, clientset, cfg)
		})
	}
}

func TestRegisterClusterWithLease_SerializesConcurrentRegistrations(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	cfg := config.LeaderElectionConfig{
		LockName:  "test-lock",
		Namespace: "default",
	}

	clientset := fake.NewClientset()

	var maxConcurrent atomic.Int32
	var currentConcurrent atomic.Int32

	registerFn := func(ctx context.Context) (*types.ClusterRegistration, error) {
		cur := currentConcurrent.Add(1)
		for {
			old := maxConcurrent.Load()
			if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond) // simulate work
		currentConcurrent.Add(-1)
		return &types.ClusterRegistration{ClusterID: "cluster-123"}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const numGoroutines = 3
	errs := make(chan error, numGoroutines)
	results := make(chan *types.ClusterRegistration, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			reg, err := RegisterClusterWithLease(ctx, log, cfg, clientset, registerFn)
			errs <- err
			results <- reg
		}()
	}

	for i := 0; i < numGoroutines; i++ {
		err := <-errs
		require.NoError(t, err)
		reg := <-results
		require.NotNil(t, reg)
		require.Equal(t, "cluster-123", reg.ClusterID)
	}

	require.Equal(t, int32(1), maxConcurrent.Load(), "registerFn should never run concurrently")
}
