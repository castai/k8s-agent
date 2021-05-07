package client

import (
	"testing"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/pointer"
)

func TestClusterNameFromTags(t *testing.T) {
	randomTag1 := &ec2.Tag{
		Key:   pointer.StringPtr("random1"),
		Value: pointer.StringPtr("value1"),
	}
	randomTag2 := &ec2.Tag{
		Key:   pointer.StringPtr("random2"),
		Value: pointer.StringPtr("value2"),
	}

	tests := []struct {
		name                string
		tags                []*ec2.Tag
		expectedClusterName string
	}{
		{
			name: "eks tag 1",
			tags: []*ec2.Tag{
				{
					Key:   pointer.StringPtr(tagEKSK8sCluster + "eks-tag-1"),
					Value: pointer.StringPtr(owned),
				},
				randomTag1,
				randomTag2,
			},
			expectedClusterName: "eks-tag-1",
		},
		{
			name: "eks tag 2",
			tags: []*ec2.Tag{
				{
					Key:   pointer.StringPtr(tagEKSK8sCluster + "eks-tag-2"),
					Value: pointer.StringPtr(owned),
				},
				randomTag1,
				randomTag2,
			},
			expectedClusterName: "eks-tag-2",
		},
		{
			name: "kops tag",
			tags: []*ec2.Tag{
				{
					Key:   pointer.StringPtr(tagKOPSKubernetesCluster),
					Value: pointer.StringPtr("kops-tag"),
				},
				randomTag1,
				randomTag2,
			},
			expectedClusterName: "kops-tag",
		},
		{
			name: "all tags",
			tags: []*ec2.Tag{
				{
					Key:   pointer.StringPtr(tagEKSK8sCluster + "all-tags"),
					Value: pointer.StringPtr(owned),
				},
				{
					Key:   pointer.StringPtr(tagEKSK8sCluster + "all-tags"),
					Value: pointer.StringPtr(owned),
				},
				{
					Key:   pointer.StringPtr(tagKOPSKubernetesCluster),
					Value: pointer.StringPtr("all-tags"),
				},
				randomTag1,
				randomTag2,
			},
			expectedClusterName: "all-tags",
		},
		{
			name: "no tags cluster tags",
			tags: []*ec2.Tag{
				randomTag1,
				randomTag2,
			},
			expectedClusterName: "",
		},
		{
			name:                "no tags at all",
			tags:                nil,
			expectedClusterName: "",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			got := getClusterName(test.tags)
			require.Equal(t, test.expectedClusterName, got)
		})
	}
}
