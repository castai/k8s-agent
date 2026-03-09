package log

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/castai/logging/components"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"

	"castai-agent/pkg/castai"
)

func TestSetupLogExporter(t *testing.T) {
	logger, hook := test.NewNullLogger()
	defer hook.Reset()
	mockClusterID := uuid.New().String()

	registrator := castai.NewRegistrator()
	registrator.ReleaseWaiters()

	manager := NewExporterManager()
	ingestClient := &mockIngestClient{}
	r := require.New(t)
	err := manager.setup(
		ingestClient,
		registrator,
		logger,
		logger,
		&Config{ExportConfig: components.Config{ClusterID: mockClusterID}, SendTimeout: time.Second},
	)
	r.NoError(err)

	t.Run("sends the log msg", func(t *testing.T) {
		r := require.New(t)

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

		entries := ingestClient.getEntries()
		r.Len(entries, 1)
		e2 := entries[0]
		fields := e2.Fields
		r.Equal(mockClusterID, fields["cluster_id"])
		r.Equal("eks", fields["provider"])
		r.Equal("false", fields["sample_boolean_value"])
		r.Equal("3", fields["int_val"])
		r.Equal("1.000000004", fields["float_val"])
	})
}

type mockIngestClient struct {
	entries []components.Entry
	mu      sync.Mutex
}

func (m *mockIngestClient) IngestLogs(ctx context.Context, entries []components.Entry) error {
	m.entries = append(m.entries, entries...)
	m.mu.Lock()
	defer m.mu.Unlock()

	return nil
}

func (m *mockIngestClient) Run(ctx context.Context) error {
	return nil
}

func (m *mockIngestClient) getEntries() []components.Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.entries)
}
