package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/sirupsen/logrus"
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

func Test_APITimeout(t *testing.T) {
	tests := map[string]struct {
		apiTimeout      time.Duration
		requestDuration time.Duration
		expectTimeout   bool
	}{
		"should fail with timeout error when request from API is not received within timeout duration": {
			apiTimeout:      10 * time.Millisecond,
			requestDuration: 50 * time.Millisecond,
			expectTimeout:   true,
		},
		"should not fail when request from API is received within timeout duration": {
			apiTimeout:      200 * time.Millisecond,
			requestDuration: 50 * time.Millisecond,
			expectTimeout:   false,
		},
	}

	for testName, test := range tests {
		test := test

		t.Run(testName, func(t *testing.T) {
			r := require.New(t)
			ctx := context.Background()
			log := logrus.New()

			withMockEC2Client := func(ctx context.Context, c *client) error {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(test.requestDuration)
					w.WriteHeader(http.StatusOK)
				}))

				s := session.Must(session.NewSession(&aws.Config{
					Region:      pointer.String("us-central1"),
					Credentials: credentials.AnonymousCredentials,
					DisableSSL:  aws.Bool(true),
					Endpoint:    aws.String(server.URL),
				}))
				c.sess = s
				c.ec2Client = ec2.New(s)

				return nil
			}

			client, err := New(ctx, log, withMockEC2Client, WithAPITimeout(test.apiTimeout))
			r.NoError(err)
			r.NotNil(client)

			_, err = client.GetInstancesByInstanceIDs(ctx, []string{"1"})
			if test.expectTimeout {
				r.Error(err)
				r.Contains(err.Error(), "RequestCanceled: request context canceled")
			} else {
				r.NoError(err)
			}
		})
	}
}
