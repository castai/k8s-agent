package log

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"

	"castai-agent/internal/castai"
	mock_castai "castai-agent/internal/castai/mock"
)

func TestSetupLogExporter(t *testing.T) {
	logger, hook := test.NewNullLogger()
	defer hook.Reset()
	mockClusterID := uuid.New().String()
	ctrl := gomock.NewController(t)
	mockapi := mock_castai.NewMockClient(ctrl)
	SetupLogExporter(logger, nil, mockapi, &Config{ClusterID: mockClusterID, SendTimeout: time.Second})

	t.Run("sends the log msg", func(t *testing.T) {
		r := require.New(t)
		mockapi.EXPECT().SendLogEvent(gomock.Any(), gomock.Any(), gomock.Any()).
			Do(assertClusterFields(r, mockClusterID, "eks")).Return(&castai.IngestAgentLogsResponse{}, nil).Times(1)
		log := logger.WithFields(logrus.Fields{
			"cluster_id": mockClusterID,
			"provider":   "eks",
		})
		log.Log(logrus.ErrorLevel, "failed to discover account id")
		time.Sleep(1 * time.Second)
	})
}

func assertClusterFields(
	r *require.Assertions, mockClusterID, provider string,
) func(_ context.Context, clusterID string, req *castai.IngestAgentLogsRequest) *castai.IngestAgentLogsResponse {
	return func(_ context.Context, clusterID string, req *castai.IngestAgentLogsRequest) *castai.IngestAgentLogsResponse {
		fields := req.LogEvent.Fields
		r.Equal(mockClusterID, fields["cluster_id"])
		r.Equal(provider, fields["provider"])
		return &castai.IngestAgentLogsResponse{}
	}
}
