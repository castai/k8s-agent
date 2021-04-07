package main

import (
	"castai-agent/internal/cast"
	mock_cast "castai-agent/internal/cast/mock"
	"castai-agent/internal/services/collector"
	mock_collector "castai-agent/internal/services/collector/mock"
	mock_providers "castai-agent/internal/services/providers/mock"
	"context"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"testing"
)

func TestCollect(t *testing.T) {
	ctx := context.Background()
	mockctrl := gomock.NewController(t)
	col := mock_collector.NewMockCollector(mockctrl)
	provider := mock_providers.NewMockProvider(mockctrl)
	clientset := fake.NewSimpleClientset()
	castclient := mock_cast.NewMockClient(mockctrl)

	c := &cast.RegisterClusterResponse{Cluster: cast.Cluster{ID: uuid.New().String(), OrganizationID: uuid.New().String()}}

	spot := v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "spot", Labels: map[string]string{}}}
	onDemand := v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "on-demand"}}

	cd := &collector.ClusterData{NodeList: &v1.NodeList{Items: []v1.Node{spot, onDemand}}}
	col.EXPECT().Collect(ctx).Return(cd, nil)

	provider.EXPECT().AccountID(ctx).Return("accountID", nil)
	provider.EXPECT().ClusterName(ctx).Return("clusterName", nil)
	provider.EXPECT().ClusterRegion(ctx).Return("eu-central-1", nil)
	provider.EXPECT().Name().Return("eks")
	provider.EXPECT().FilterSpot(ctx, []*v1.Node{&spot, &onDemand}).Return([]*v1.Node{&spot}, nil)

	castclient.EXPECT().SendClusterSnapshot(ctx, &cast.Snapshot{
		ClusterID:       c.Cluster.ID,
		OrganizationID:  c.Cluster.OrganizationID,
		AccountID:       "accountID",
		ClusterProvider: "EKS",
		ClusterName:     "clusterName",
		ClusterRegion:   "eu-central-1",
		ClusterData:     cd,
	}).Return(nil)

	err := collect(ctx, logrus.New(), c, col, provider, clientset, castclient)

	require.NoError(t, err)

	require.Equal(t, map[string]string{"scheduling.cast.ai/spot": "true"}, spot.ObjectMeta.Labels)
}
