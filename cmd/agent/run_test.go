package agent

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	mock_castai "castai-agent/mocks/pkg_/castai"
	mock_provider "castai-agent/mocks/pkg_/services/providers/types"
	"castai-agent/pkg/services/providers/types"
)

func Test_waitForLeadershipAndRegister(t *testing.T) {
	log := logrus.NewEntry(logrus.New())

	t.Run("registers cluster when leadership is signalled", func(t *testing.T) {
		provider := mock_provider.NewMockProvider(t)
		client := mock_castai.NewMockClient(t)

		leaderAcquiredCh := make(chan struct{})

		expectedReg := &types.ClusterRegistration{
			ClusterID:      "cluster-123",
			OrganizationID: "org-456",
		}
		provider.EXPECT().RegisterCluster(mock.Anything, client).Return(expectedReg, nil).Once()

		// Signal leadership after a short delay
		go func() {
			time.Sleep(50 * time.Millisecond)
			close(leaderAcquiredCh)
		}()

		reg, err := waitForLeadershipAndRegister(context.Background(), log, leaderAcquiredCh, provider, client)

		require.NoError(t, err)
		require.Equal(t, "cluster-123", reg.ClusterID)
		require.Equal(t, "org-456", reg.OrganizationID)
	})

	t.Run("registers immediately when channel is already closed", func(t *testing.T) {
		provider := mock_provider.NewMockProvider(t)
		client := mock_castai.NewMockClient(t)

		leaderAcquiredCh := make(chan struct{})
		close(leaderAcquiredCh)

		expectedReg := &types.ClusterRegistration{
			ClusterID: "cluster-789",
		}
		provider.EXPECT().RegisterCluster(mock.Anything, client).Return(expectedReg, nil).Once()

		reg, err := waitForLeadershipAndRegister(context.Background(), log, leaderAcquiredCh, provider, client)

		require.NoError(t, err)
		require.Equal(t, "cluster-789", reg.ClusterID)
	})

	t.Run("returns error on context cancellation", func(t *testing.T) {
		provider := mock_provider.NewMockProvider(t)
		client := mock_castai.NewMockClient(t)

		leaderAcquiredCh := make(chan struct{}) // never closed

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		reg, err := waitForLeadershipAndRegister(ctx, log, leaderAcquiredCh, provider, client)

		require.Error(t, err)
		require.Nil(t, reg)
	})

	t.Run("returns error when registration fails", func(t *testing.T) {
		provider := mock_provider.NewMockProvider(t)
		client := mock_castai.NewMockClient(t)

		leaderAcquiredCh := make(chan struct{})
		close(leaderAcquiredCh)

		provider.EXPECT().RegisterCluster(mock.Anything, client).Return(nil, context.DeadlineExceeded).Once()

		reg, err := waitForLeadershipAndRegister(context.Background(), log, leaderAcquiredCh, provider, client)

		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Nil(t, reg)
	})
}
