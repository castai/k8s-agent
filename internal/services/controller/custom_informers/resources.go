package custom_informers

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/informers/internalinterfaces"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"k8s.io/metrics/pkg/client/clientset/versioned"
)

type PodMetricsInformer interface {
	Informer() cache.SharedIndexInformer
}

func NewFilteredPodMetricsInformer(client *versioned.Clientset, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}

				return client.MetricsV1beta1().PodMetricses(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}

				return client.MetricsV1beta1().PodMetricses(namespace).Watch(context.TODO(), options)
			},
		},
		&v1beta1.PodMetrics{},
		resyncPeriod,
		indexers,
	)
}

func NewDefaultInformer(config *rest.Config, namespace string) func(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return func(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
		metricsClient := versioned.NewForConfigOrDie(config)
		return NewFilteredPodMetricsInformer(metricsClient, namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, nil)
	}
}
