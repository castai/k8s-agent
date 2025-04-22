package metadata

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakeclientset "k8s.io/client-go/kubernetes/fake"

	"castai-agent/internal/config"
)

func TestStoreImpl_StoreMetadataConfigMap(t *testing.T) {
	tests := []struct {
		name              string
		cfg               config.Config
		metadata          *Metadata
		existingConfigMap *corev1.ConfigMap
		expectedError     bool
	}{
		{
			name: "should store metadata successfully when config map does not exist",
			cfg: config.Config{
				MetadataStore: &config.MetadataStoreConfig{
					Enabled:            true,
					ConfigMapName:      "castai-agent-metadata",
					ConfigMapNamespace: "default",
				},
			},
			metadata: &Metadata{
				ClusterID: "test-cluster-id",
			},
			expectedError: false,
		},
		{
			name: "should store metadata successfully when config map exists",
			cfg: config.Config{
				MetadataStore: &config.MetadataStoreConfig{
					Enabled:            true,
					ConfigMapName:      "castai-agent-metadata",
					ConfigMapNamespace: "default",
				},
			},
			metadata: &Metadata{
				ClusterID: "test-cluster-id",
			},
			existingConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "castai-agent-metadata",
					Namespace: "default",
				},
				Data: map[string]string{
					"CLUSTER_ID": "original-test-cluster-id",
				},
			},
			expectedError: false,
		},
		{
			name: "should not store metadata when store is disabled",
			cfg: config.Config{
				MetadataStore: &config.MetadataStoreConfig{
					Enabled: false,
				},
			},
			metadata: &Metadata{
				ClusterID: "test-cluster-id",
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := require.New(t)
			clientset := fakeclientset.NewSimpleClientset()
			store := New(clientset, tt.cfg)

			if tt.existingConfigMap != nil {
				_, err := clientset.CoreV1().ConfigMaps(tt.cfg.MetadataStore.ConfigMapNamespace).Create(context.Background(), tt.existingConfigMap, metav1.CreateOptions{})
				r.NoError(err)
			}

			err := store.StoreMetadataConfigMap(context.Background(), tt.metadata)
			if tt.expectedError {
				r.Error(err)
			} else {
				r.NoError(err)
				if tt.cfg.MetadataStore.Enabled {
					configMap, err := clientset.CoreV1().ConfigMaps(tt.cfg.MetadataStore.ConfigMapNamespace).Get(context.Background(), tt.cfg.MetadataStore.ConfigMapName, metav1.GetOptions{})
					r.NoError(err)
					r.Equal(tt.metadata.ClusterID, configMap.Data["CLUSTER_ID"])
				}
			}
		})
	}
}
