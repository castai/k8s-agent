package metadata

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

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
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.cfg.MetadataStore.ConfigMapName,
			Namespace: configMapNamespace,
		},
		Data: map[string]string{
			"CLUSTER_ID": metadata.ClusterID,
		},
	}

	_, err := s.clientset.CoreV1().ConfigMaps(configMapNamespace).Update(ctx, configMap, metav1.UpdateOptions{})
	if errors.IsNotFound(err) {
		_, err = s.clientset.CoreV1().ConfigMaps(configMapNamespace).Create(ctx, configMap, metav1.CreateOptions{})
	}

	return err
}
