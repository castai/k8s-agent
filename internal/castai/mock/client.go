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

// SendClusterSnapshot mocks base method.
func (m *MockClient) SendClusterSnapshot(ctx context.Context, snap *castai.Snapshot) (*castai.SnapshotResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SendClusterSnapshot", arg0, arg1)
	ret0, _ := ret[0].(error)
	return
}

// SendClusterSnapshot indicates an expected call of SendClusterSnapshot.
func (mr *MockClientMockRecorder) SendClusterSnapshot(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SendClusterSnapshot", reflect.TypeOf((*MockClient)(nil).SendClusterSnapshot), arg0, arg1)
}
