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

	registrator := castai.NewRegistrator()
	registrator.ReleaseWaiters()

	SetupLogExporter(registrator, logger, nil, mockapi, &Config{ClusterID: mockClusterID, SendTimeout: time.Second})

	t.Run("sends the log msg", func(t *testing.T) {
		r := require.New(t)

		mockapi.EXPECT().SendLogEvent(gomock.Any(), gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, clusterID string, req *castai.IngestAgentLogsRequest) *castai.IngestAgentLogsResponse {
				fields := req.LogEvent.Fields
				r.Equal(mockClusterID, fields["cluster_id"])
				r.Equal("eks", fields["provider"])
				r.Equal("false", fields["sample_boolean_value"])
				r.Equal("3", fields["int_val"])
				r.Equal("1.000000004", fields["float_val"])
				return &castai.IngestAgentLogsResponse{}
			}).Return(&castai.IngestAgentLogsResponse{}, nil).Times(1)

		log := logger.WithFields(logrus.Fields{
			"cluster_id": mockClusterID,
			"provider":   "eks",
			// log interface allows not just the strings - must make sure we correctly convert them to strings when sending
			"sample_boolean_value": false,
			"int_val":              3,
			"float_val":            1.000000004,
		})
		log.Log(logrus.ErrorLevel, "failed to discover account id")
		time.Sleep(1 * time.Second)
	})
}
