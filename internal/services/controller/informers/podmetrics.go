package informers

import (
	"context"
	"time"

	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"k8s.io/metrics/pkg/client/clientset/versioned"
	"k8s.io/utils/clock"

	"castai-agent/internal/services/metrics"
)

const fetchInterval = 30 * time.Second

type PodMetricsInformer interface {
	Informer() cache.SharedIndexInformer
}

type metricsWatch struct {
	resultChan    chan watch.Event
	closeChan     chan struct{}
	client        versioned.Interface
	log           logrus.FieldLogger
	listOptions   metav1.ListOptions
	fetchInterval time.Duration
}

func NewMetricsWatch(
	ctx context.Context,
	log logrus.FieldLogger,
	client versioned.Interface,
	listOptions metav1.ListOptions,
) watch.Interface {
	metrics := newMetricsWatch(log, client, listOptions, fetchInterval)

	go metrics.Start(ctx)

	return metrics
}

func newMetricsWatch(log logrus.FieldLogger,
	client versioned.Interface,
	listOptions metav1.ListOptions,
	fetchInterval time.Duration) *metricsWatch {
	return &metricsWatch{
		closeChan:     make(chan struct{}),
		resultChan:    make(chan watch.Event),
		log:           log,
		client:        client,
		listOptions:   withDefaultTimeout(listOptions),
		fetchInterval: fetchInterval,
	}
}

func (m *metricsWatch) Start(ctx context.Context) {
	m.log.Infof("Starting pod metrics polling")

	backoff := wait.NewExponentialBackoffManager(m.fetchInterval, 5*time.Minute, m.fetchInterval, 2, 0.2, clock.RealClock{})
	wait.BackoffUntil(func() {
		result, err := m.client.MetricsV1beta1().
			PodMetricses("").
			List(ctx, m.listOptions)
		if err != nil {
			m.log.Infof("Failed getting pod metrics, check metrics server health: %v", err.Error())
			return
		}

		metrics.WatchReceived.WithLabelValues("*v1beta1.PodMetrics", "update").Add(float64(len(result.Items)))

		for _, metrics := range result.Items {
			metrics := metrics
			// Skip labels, as we already have that in the pod list.
			metrics.Labels = map[string]string{}

			select {
			case <-ctx.Done():
				return
			case <-m.closeChan:
				close(m.resultChan)
				return
			default:
				m.resultChan <- watch.Event{
					Type:   watch.Modified,
					Object: &metrics,
				}
			}
		}
	}, backoff, true, m.closeChan)
	m.log.Infof("Stopped pod metrics polling")
}

func (m *metricsWatch) Stop() {
	m.log.Infof("Stopping pod metrics polling")
	close(m.closeChan)
}

func (m *metricsWatch) ResultChan() <-chan watch.Event {
	return m.resultChan
}

func NewPodMetricsInformer(log logrus.FieldLogger, client versioned.Interface, tweakListOptions func(options *metav1.ListOptions)) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				options = withDefaultTimeout(options)
				tweakListOptions(&options)

				return client.MetricsV1beta1().
					PodMetricses("").
					List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return NewMetricsWatch(context.TODO(), log, client, options), nil
			},
			DisableChunking: true,
		},
		&v1beta1.PodMetrics{},
		time.Second*30,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)
}

func withDefaultTimeout(options metav1.ListOptions) metav1.ListOptions {
	if options.TimeoutSeconds == nil {
		options.TimeoutSeconds = lo.ToPtr(int64(15))
	}

	return options
}
