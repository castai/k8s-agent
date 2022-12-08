package informers

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"k8s.io/metrics/pkg/client/clientset/versioned"
	"k8s.io/utils/clock"
)

type PodMetricsInformer interface {
	Informer() cache.SharedIndexInformer
}

type metricsWatch struct {
	resultChan chan watch.Event
	client     versioned.Interface
	log        logrus.FieldLogger
}

func NewMetricsWatch(ctx context.Context, log logrus.FieldLogger, client versioned.Interface) watch.Interface {
	metrics := &metricsWatch{
		resultChan: make(chan watch.Event),
		log:        log,
		client:     client,
	}

	go metrics.Start(ctx)

	return metrics
}

func (m *metricsWatch) Start(ctx context.Context) {
	m.log.Infof("Starting pod metrics polling")
	const fetchInterval = 30 * time.Second
	backoff := wait.NewExponentialBackoffManager(fetchInterval, 5*time.Minute, fetchInterval, 2, 0.2, clock.RealClock{})
	wait.BackoffUntil(func() {
		result, err := m.client.MetricsV1beta1().
			PodMetricses("").
			List(ctx, metav1.ListOptions{})
		if err != nil {
			m.log.Infof("Failed getting pod metrics, check metrics server health: %v", err.Error())
			return
		}

		for _, metrics := range result.Items {
			metrics := metrics
			// Skip labels, as we already have that in the pod list.
			metrics.Labels = map[string]string{}

			select {
			case <-ctx.Done():
				return
			case m.resultChan <- watch.Event{
				Type:   watch.Modified,
				Object: &metrics,
			}:
			}
		}
	}, backoff, true, ctx.Done())
}

func (m *metricsWatch) Stop() {
	close(m.resultChan)
}

func (m *metricsWatch) ResultChan() <-chan watch.Event {
	return m.resultChan
}

func NewPodMetricsInformer(log logrus.FieldLogger, client versioned.Interface) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return client.MetricsV1beta1().PodMetricses("").List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return NewMetricsWatch(context.TODO(), log, client), nil
			},
			DisableChunking: true,
		},
		&v1beta1.PodMetrics{},
		time.Second*30,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)
}
