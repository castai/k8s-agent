package monitor

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
)

func Run(ctx context.Context, log *logrus.Entry, clientset *kubernetes.Clientset) error {
	m := monitor{
		clientset: clientset,
		log:       log,
	}

	if err := m.waitForClusterID(ctx); err != nil {
		return fmt.Errorf("waiting for cluster ID: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second * 5):
		}

		if err := m.runChecks(ctx); err != nil {
			return fmt.Errorf("running monitor checks: %w", err)
		}

		_ = clientset
	}
}

type monitor struct {
	clientset *kubernetes.Clientset
	log       *logrus.Entry
}

func (m *monitor) waitForClusterID(_ context.Context) error {
	return nil
}

func (m *monitor) runChecks(_ context.Context) error {
	m.log.Infof("running checks")
	return nil
}
