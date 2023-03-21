package informers

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	fake_metrics "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

func Test_metricsWatch(t *testing.T) {
	t.Run("should not panic when metrics watch is started after Stop is called", func(t *testing.T) {
		ctx := context.Background()
		log := logrus.New()

		metrics := &v1beta1.PodMetricsList{Items: []v1beta1.PodMetrics{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "p1",
					Namespace: "",
				},
				Containers: []v1beta1.ContainerMetrics{
					{
						Name: "container1",
						Usage: v1.ResourceList{
							"memory": resource.MustParse("10Gi"),
							"cpu":    resource.MustParse("10"),
						},
					},
				},
			},
		}}

		metricsClient := fake_metrics.NewSimpleClientset()
		metricsClient.PrependReactor("*", "*", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, metrics, nil
		})

		w := newMetricsWatch(log, metricsClient, metav1.ListOptions{}, time.Second)

		// Manually stop the watch before starting the watch.
		w.Stop()
		go w.Start(ctx)

	read:
		for {
			select {
			case <-w.ResultChan():
				continue
			default:
				break read
			}
		}

		<-time.After(time.Second * 2)
	})
}
