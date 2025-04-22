package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	_ "k8s.io/component-base/metrics/prometheus/clientgo" // client-go metrics registration
)

const (
	namespace      = "castai"
	subsystem      = "agent"
	labelType      = "type"
	labelEventType = "event_type"
)

var (
	Registry      = prometheus.NewRegistry()
	WatchReceived = promauto.With(Registry).NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "watch_received_total",
		Help:      "Number of received watch events.",
	}, []string{labelType, labelEventType})

	CacheSize = promauto.With(Registry).NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "cache_size_count",
		Help:      "Number of items in the current send cache.",
	})

	CacheLatency = promauto.With(Registry).NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "cache_latency_milliseconds",
		Help:      "Latency of the cache in milliseconds.",
		Buckets:   prometheus.ExponentialBucketsRange(10, 60000, 25),
	})

	DeltaSendTime = promauto.With(Registry).NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "delta_send_seconds",
		Help:      "Seconds taken to send delta.",
		Buckets:   prometheus.ExponentialBucketsRange(1, 300, 25),
	})

	DeltaSendInterval = promauto.With(Registry).NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "delta_send_interval_seconds",
		Help:      "Interval between delta sends in seconds.",
		Buckets:   prometheus.ExponentialBucketsRange(1, 300, 25),
	})
)
