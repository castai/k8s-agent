package log

import (
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"

	mock_castai "castai-agent/internal/castai/mock"
)

func TestSetupLogExporter(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "exports logs from the client",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			logger, hook := test.NewNullLogger()
			defer hook.Reset()
			ctrl := gomock.NewController(t)
			clusterID := uuid.New().String()
			mockapi := mock_castai.NewMockClient(ctrl)
			SetupLogExporter(logger, mockapi, Config{
				ClusterID:          clusterID,
				MsgSendTimeoutSecs: 0,
			})


			time.Sleep(1 * time.Second)

			logger.Log(logrus.ErrorLevel, "failed to discover account id")
			mockapi.EXPECT().SendLogEvent(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
		})
	}
}
