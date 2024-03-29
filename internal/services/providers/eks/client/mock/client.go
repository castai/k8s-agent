// Code generated by MockGen. DO NOT EDIT.
// Source: castai-agent/internal/services/providers/eks/client (interfaces: Client)

// Package mock_client is a generated GoMock package.
package mock_client

import (
	context "context"
	reflect "reflect"

	ec2 "github.com/aws/aws-sdk-go/service/ec2"
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

// GetAccountID mocks base method.
func (m *MockClient) GetAccountID(arg0 context.Context) (*string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetAccountID", arg0)
	ret0, _ := ret[0].(*string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetAccountID indicates an expected call of GetAccountID.
func (mr *MockClientMockRecorder) GetAccountID(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetAccountID", reflect.TypeOf((*MockClient)(nil).GetAccountID), arg0)
}

// GetClusterName mocks base method.
func (m *MockClient) GetClusterName(arg0 context.Context) (*string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetClusterName", arg0)
	ret0, _ := ret[0].(*string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetClusterName indicates an expected call of GetClusterName.
func (mr *MockClientMockRecorder) GetClusterName(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetClusterName", reflect.TypeOf((*MockClient)(nil).GetClusterName), arg0)
}

// GetInstancesByInstanceIDs mocks base method.
func (m *MockClient) GetInstancesByInstanceIDs(arg0 context.Context, arg1 []string) ([]*ec2.Instance, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetInstancesByInstanceIDs", arg0, arg1)
	ret0, _ := ret[0].([]*ec2.Instance)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetInstancesByInstanceIDs indicates an expected call of GetInstancesByInstanceIDs.
func (mr *MockClientMockRecorder) GetInstancesByInstanceIDs(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetInstancesByInstanceIDs", reflect.TypeOf((*MockClient)(nil).GetInstancesByInstanceIDs), arg0, arg1)
}

// GetRegion mocks base method.
func (m *MockClient) GetRegion(arg0 context.Context) (*string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetRegion", arg0)
	ret0, _ := ret[0].(*string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetRegion indicates an expected call of GetRegion.
func (mr *MockClientMockRecorder) GetRegion(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetRegion", reflect.TypeOf((*MockClient)(nil).GetRegion), arg0)
}
