// Code generated by MockGen. DO NOT EDIT.
// Source: castai-agent/internal/services/providers (interfaces: Provider)

// Package mock_providers is a generated GoMock package.
package mock_providers

import (
	cast "castai-agent/internal/cast"
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	v1 "k8s.io/api/core/v1"
)

// MockProvider is a mock of Provider interface.
type MockProvider struct {
	ctrl     *gomock.Controller
	recorder *MockProviderMockRecorder
}

// MockProviderMockRecorder is the mock recorder for MockProvider.
type MockProviderMockRecorder struct {
	mock *MockProvider
}

// NewMockProvider creates a new mock instance.
func NewMockProvider(ctrl *gomock.Controller) *MockProvider {
	mock := &MockProvider{ctrl: ctrl}
	mock.recorder = &MockProviderMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockProvider) EXPECT() *MockProviderMockRecorder {
	return m.recorder
}

// AccountID mocks base method.
func (m *MockProvider) AccountID(arg0 context.Context) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AccountID", arg0)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// AccountID indicates an expected call of AccountID.
func (mr *MockProviderMockRecorder) AccountID(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AccountID", reflect.TypeOf((*MockProvider)(nil).AccountID), arg0)
}

// ClusterName mocks base method.
func (m *MockProvider) ClusterName(arg0 context.Context) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ClusterName", arg0)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ClusterName indicates an expected call of ClusterName.
func (mr *MockProviderMockRecorder) ClusterName(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ClusterName", reflect.TypeOf((*MockProvider)(nil).ClusterName), arg0)
}

// ClusterRegion mocks base method.
func (m *MockProvider) ClusterRegion(arg0 context.Context) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ClusterRegion", arg0)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ClusterRegion indicates an expected call of ClusterRegion.
func (mr *MockProviderMockRecorder) ClusterRegion(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ClusterRegion", reflect.TypeOf((*MockProvider)(nil).ClusterRegion), arg0)
}

// FilterSpot mocks base method.
func (m *MockProvider) FilterSpot(arg0 context.Context, arg1 []*v1.Node) ([]*v1.Node, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FilterSpot", arg0, arg1)
	ret0, _ := ret[0].([]*v1.Node)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// FilterSpot indicates an expected call of FilterSpot.
func (mr *MockProviderMockRecorder) FilterSpot(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FilterSpot", reflect.TypeOf((*MockProvider)(nil).FilterSpot), arg0, arg1)
}

// Name mocks base method.
func (m *MockProvider) Name() string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Name")
	ret0, _ := ret[0].(string)
	return ret0
}

// Name indicates an expected call of Name.
func (mr *MockProviderMockRecorder) Name() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Name", reflect.TypeOf((*MockProvider)(nil).Name))
}

// RegisterClusterRequest mocks base method.
func (m *MockProvider) RegisterClusterRequest(arg0 context.Context) (*cast.RegisterClusterRequest, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RegisterClusterRequest", arg0)
	ret0, _ := ret[0].(*cast.RegisterClusterRequest)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// RegisterClusterRequest indicates an expected call of RegisterClusterRequest.
func (mr *MockProviderMockRecorder) RegisterClusterRequest(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RegisterClusterRequest", reflect.TypeOf((*MockProvider)(nil).RegisterClusterRequest), arg0)
}
