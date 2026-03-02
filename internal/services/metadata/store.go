package metadata

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	corev1 "k8s.io/api/core/v1"

	"castai-agent/internal/config"
)

type Store interface {
	// StoreMetadataConfigMap stores relevant agent runtime metadata in a config map.
	StoreMetadataConfigMap(ctx context.Context, metadata *Metadata) error
}

var _ Store = (*StoreImpl)(nil)

type StoreImpl struct {
	clientset kubernetes.Interface
	cfg       config.Config
}

func New(clientset kubernetes.Interface, cfg config.Config) *StoreImpl {
	return &StoreImpl{
		clientset: clientset,
		cfg:       cfg,
	}
}

func (s *StoreImpl) StoreMetadataConfigMap(ctx context.Context, metadata *Metadata) error {
	if !s.cfg.MetadataStore.Enabled {
		return nil
	}

	configMapNamespace := s.cfg.MetadataStore.ConfigMapNamespace
	configMapName := s.cfg.MetadataStore.ConfigMapName

	// Use strategic merge patch to only set CLUSTER_ID without overwriting
	// other keys (e.g. API_URL, GRPC_URL) that may have been set by Helm.
	patch := map[string]interface{}{
		"data": map[string]string{
			"CLUSTER_ID": metadata.ClusterID,
		},
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshaling configmap patch: %w", err)
	}

	_, err = s.clientset.CoreV1().ConfigMaps(configMapNamespace).Patch(
		ctx, configMapName, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{},
	)
	if errors.IsNotFound(err) {
		// ConfigMap doesn't exist yet (e.g. Helm-managed CM was not installed); create it.
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: configMapNamespace,
			},
			Data: map[string]string{
				"CLUSTER_ID": metadata.ClusterID,
			},
		}
		_, err = s.clientset.CoreV1().ConfigMaps(configMapNamespace).Create(ctx, configMap, metav1.CreateOptions{})
	}

	return err
}
