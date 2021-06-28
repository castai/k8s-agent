// Code generated by MockGen. DO NOT EDIT.
// Source: castai-agent/internal/castai (interfaces: Client)

// Package mock_castai is a generated GoMock package.
package mock_castai

import (
	castai "castai-agent/internal/castai"
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
)

// MockClient is a mock of Client interface.
type MockClient struct {
	ctrl     *gomock.Controller
	recorder *MockClientMockRecorder
}

// MockClientMockRecorder is the mock recorder for MockClient.
type MockClientMockRecorder struct {
	mock *MockClient
}

// NewMockClient creates a new mock instance.
func NewMockClient(ctrl *gomock.Controller) *MockClient {
	mock := &MockClient{ctrl: ctrl}
	mock.recorder = &MockClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockClient) EXPECT() *MockClientMockRecorder {
	return m.recorder
}

// ExchangeAgentTelemetry mocks base method.
func (m *MockClient) ExchangeAgentTelemetry(arg0 context.Context, arg1 string, arg2 *castai.AgentTelemetryRequest) (*castai.AgentTelemetryResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ExchangeAgentTelemetry", arg0, arg1, arg2)
	ret0, _ := ret[0].(*castai.AgentTelemetryResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ExchangeAgentTelemetry indicates an expected call of ExchangeAgentTelemetry.
func (mr *MockClientMockRecorder) ExchangeAgentTelemetry(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ExchangeAgentTelemetry", reflect.TypeOf((*MockClient)(nil).ExchangeAgentTelemetry), arg0, arg1, arg2)
}

// RegisterCluster mocks base method.
func (m *MockClient) RegisterCluster(arg0 context.Context, arg1 *castai.RegisterClusterRequest) (*castai.RegisterClusterResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RegisterCluster", arg0, arg1)
	ret0, _ := ret[0].(*castai.RegisterClusterResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// RegisterCluster indicates an expected call of RegisterCluster.
func (mr *MockClientMockRecorder) RegisterCluster(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RegisterCluster", reflect.TypeOf((*MockClient)(nil).RegisterCluster), arg0, arg1)
}

// SendDelta mocks base method.
func (m *MockClient) SendDelta(arg0 context.Context, arg1 *castai.Delta) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SendDelta", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// SendDelta indicates an expected call of SendDelta.
func (mr *MockClientMockRecorder) SendDelta(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SendDelta", reflect.TypeOf((*MockClient)(nil).SendDelta), arg0, arg1)
}

// SendLogEvent mocks base method.
func (m *MockClient) SendLogEvent(arg0 context.Context, arg1 string, arg2 *castai.IngestAgentLogsRequest) *castai.IngestAgentLogsResponse {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SendLogEvent", arg0, arg1, arg2)
	ret0, _ := ret[0].(*castai.IngestAgentLogsResponse)
	return ret0
}

// SendLogEvent indicates an expected call of SendLogEvent.
func (mr *MockClientMockRecorder) SendLogEvent(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SendLogEvent", reflect.TypeOf((*MockClient)(nil).SendLogEvent), arg0, arg1, arg2)
}
