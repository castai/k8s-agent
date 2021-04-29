package main

import (
	"context"
	"testing"

	"castai-agent/internal/castai"
	mock_castai "castai-agent/internal/castai/mock"
	"castai-agent/internal/services/collector"
	mock_collector "castai-agent/internal/services/collector/mock"
	"castai-agent/internal/services/providers/types"
	mock_types "castai-agent/internal/services/providers/types/mock"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
)

func TestCollect(t *testing.T) {
	ctx := context.Background()
	mockctrl := gomock.NewController(t)
	col := mock_collector.NewMockCollector(mockctrl)
	provider := mock_types.NewMockProvider(mockctrl)
	castclient := mock_castai.NewMockClient(mockctrl)

	reg := &types.ClusterRegistration{
		ClusterID:      uuid.New().String(),
		OrganizationID: uuid.New().String(),
	}

	spot := v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "spot", Labels: map[string]string{}}}
	onDemand := v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "on-demand"}}

	cd := &collector.ClusterData{NodeList: &v1.NodeList{Items: []v1.Node{spot, onDemand}}}
	col.EXPECT().Collect(ctx).Return(cd, nil)
	col.EXPECT().GetVersion().Return(&version.Info{Major: "1", Minor: "20"})

	provider.EXPECT().AccountID(ctx).Return("accountID", nil)
	provider.EXPECT().ClusterName(ctx).Return("clusterName", nil)
	provider.EXPECT().ClusterRegion(ctx).Return("eu-central-1", nil)
	provider.EXPECT().Name().Return("eks")
	provider.EXPECT().FilterSpot(ctx, []*v1.Node{&spot, &onDemand}).Return([]*v1.Node{&spot}, nil)

	castclient.EXPECT().SendClusterSnapshot(ctx, &castai.Snapshot{
		ClusterID:       reg.ClusterID,
		OrganizationID:  reg.OrganizationID,
		AccountID:       "accountID",
		ClusterProvider: "EKS",
		ClusterName:     "clusterName",
		ClusterRegion:   "eu-central-1",
		ClusterData:     cd,
		ClusterVersion:  "1.20",
	}).Return(nil)

	_, err := collect(ctx, logrus.New(), reg, col, provider, castclient)

	require.NoError(t, err)

	require.Equal(t, map[string]string{"scheduling.cast.ai/spot": "true"}, spot.ObjectMeta.Labels)
}
