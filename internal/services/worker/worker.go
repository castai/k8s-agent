package worker

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"castai-agent/internal/castai"
	"castai-agent/internal/services/collector"
	"castai-agent/internal/services/providers/types"
	"castai-agent/pkg/labels"
)

func Run(
	ctx context.Context,
	log logrus.FieldLogger,
	reg *types.ClusterRegistration,
	col collector.Collector,
	castclient castai.Client,
	provider types.Provider,
) error {
	w := &worker{
		log:        log,
		col:        col,
		castclient: castclient,
		provider:   provider,
		reg:        reg,
		intervalCh: make(chan struct{}),
	}

	return w.run(ctx)
}

type worker struct {
	log        logrus.FieldLogger
	col        collector.Collector
	castclient castai.Client
	provider   types.Provider
	reg        *types.ClusterRegistration
	interval   time.Duration
	intervalCh chan struct{}
}

func (w *worker) run(ctx context.Context) error {
	interval, err := w.getInterval(ctx)
	if err != nil {
		return fmt.Errorf("getting snapshot collection interval: %w", err)
	}

	w.interval = *interval

	if err := w.collect(ctx); err != nil {
		return fmt.Errorf("collecting first snapshot data: %w", err)
	}

	go w.pollInterval(ctx)

	w.log.Infof("collecting snapshot every %s", w.interval)

	for {
		select {
		case <-time.After(w.interval):
		case <-w.intervalCh:
		case <-ctx.Done():
			return nil
		}

		if err := w.collect(ctx); err != nil {
			w.log.Errorf("collecting snapshot data: %v", err)
		}
	}
}

func (w *worker) pollInterval(ctx context.Context) {
	const dur = 15 * time.Second
	w.log.Infof("polling agent configuration every %s", dur)
	wait.Until(func() {
		interval, err := w.getInterval(ctx)
		if err != nil {
			w.log.Errorf("polling interval: %v", err)
			return
		}
		if *interval != w.interval {
			w.log.Infof("snapshot collection interval changed from %s to %s", w.interval, *interval)
			w.interval = *interval
			w.intervalCh <- struct{}{}
		}
	}, dur, ctx.Done())
}

func (w *worker) getInterval(ctx context.Context) (*time.Duration, error) {
	cfg, err := w.castclient.GetAgentCfg(ctx, w.reg.ClusterID)
	if err != nil {
		return nil, fmt.Errorf("getting agent configuration: %w", err)
	}

	intervalSeconds, err := strconv.Atoi(cfg.IntervalSeconds)
	if err != nil {
		return nil, fmt.Errorf("parsing interval %q: %w", cfg.IntervalSeconds, err)
	}

	remoteInterval := time.Duration(intervalSeconds) * time.Second

	return &remoteInterval, nil
}

func (w *worker) collect(ctx context.Context) error {
	cd, err := w.col.Collect(ctx)
	if err != nil {
		return err
	}

	accountID, err := w.provider.AccountID(ctx)
	if err != nil {
		return fmt.Errorf("getting account id: %w", err)
	}

	clusterName, err := w.provider.ClusterName(ctx)
	if err != nil {
		return fmt.Errorf("getting cluster name: %w", err)
	}

	region, err := w.provider.ClusterRegion(ctx)
	if err != nil {
		return fmt.Errorf("getting cluster region: %w", err)
	}

	snap := &castai.Snapshot{
		ClusterID:       w.reg.ClusterID,
		OrganizationID:  w.reg.OrganizationID,
		ClusterProvider: strings.ToUpper(w.provider.Name()),
		AccountID:       accountID,
		ClusterName:     clusterName,
		ClusterRegion:   region,
		ClusterData:     cd,
	}

	if v := w.col.GetVersion(); v != nil {
		snap.ClusterVersion = v.Major + "." + v.Minor
	}

	if err := w.addSpotLabel(ctx, snap.NodeList); err != nil {
		w.log.Errorf("adding spot labels: %v", err)
	}

	ctx, cancel := context.WithTimeout(ctx, w.interval)
	defer cancel()

	if err := w.castclient.SendClusterSnapshotWithRetry(ctx, snap); err != nil {
		return fmt.Errorf("sending cluster snapshot: %w", err)
	}

	return nil
}

func (w *worker) addSpotLabel(ctx context.Context, nodes *v1.NodeList) error {
	nodeMap := make(map[string]*v1.Node, len(nodes.Items))
	items := make([]*v1.Node, len(nodes.Items))
	for i, node := range nodes.Items {
		items[i] = &nodes.Items[i]
		nodeMap[node.Name] = &nodes.Items[i]
	}

	spotNodes, err := w.provider.FilterSpot(ctx, items)
	if err != nil {
		return fmt.Errorf("filtering spot instances: %w", err)
	}

	for _, node := range spotNodes {
		nodeMap[node.Name].Labels[labels.Spot] = "true"
	}

	return nil
}
