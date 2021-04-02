package collector

import (
	"context"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Collector struct {
	log       logrus.FieldLogger
	clientset kubernetes.Interface
	cd        *ClusterData
}

func NewCollector(log logrus.FieldLogger, clientset kubernetes.Interface) *Collector {
	var cd ClusterData
	return &Collector{
		log:       log,
		clientset: clientset,
		cd:        &cd,
	}
}

func (c *Collector) Collect(ctx context.Context) (*ClusterData, error) {
	if err := c.collectNodes(ctx); err != nil {
		return nil, err
	}

	if err := c.collectPods(ctx); err != nil {
		return nil, err
	}

	if err := c.collectPods(ctx); err != nil {
		return nil, err
	}

	if err := c.collectPersistentVolumes(ctx); err != nil {
		return nil, err
	}

	if err := c.collectPersistentVolumeClaims(ctx); err != nil {
		return nil, err
	}

	if err := c.collectDeploymentList(ctx); err != nil {
		return nil, err
	}

	if err := c.collectReplicaSetList(ctx); err != nil {
		return nil, err
	}

	if err := c.collectDaemonSetList(ctx); err != nil {
		return nil, err
	}

	if err := c.collectStatefulSetList(ctx); err != nil {
		return nil, err
	}

	if err := c.collectReplicationControllerList(ctx); err != nil {
		return nil, err
	}

	if err := c.collectServiceList(ctx); err != nil {
		return nil, err
	}

	if err := c.collectCSINodeList(ctx); err != nil {
		return nil, err
	}

	if err := c.collectStorageClassList(ctx); err != nil {
		return nil, err
	}

	if err := c.collectJobList(ctx); err != nil {
		return nil, err
	}

	return c.cd, nil
}

func (c *Collector) collectNodes(ctx context.Context) error {
	nodes, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.NodeList = nodes
	return nil
}

func (c *Collector) collectPods(ctx context.Context) error {
	pods, err := c.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.PodList = pods
	return nil
}

func (c *Collector) collectPersistentVolumes(ctx context.Context) error {
	pods, err := c.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.PodList = pods
	return nil
}

func (c *Collector) collectPersistentVolumeClaims(ctx context.Context) error {
	pvc, err := c.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.PersistentVolumeClaimList = pvc
	return nil
}

func (c *Collector) collectDeploymentList(ctx context.Context) error {
	dpls, err := c.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.DeploymentList = dpls
	return nil
}

func (c *Collector) collectReplicaSetList(ctx context.Context) error {
	rpsl, err := c.clientset.AppsV1().ReplicaSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.ReplicaSetList = rpsl
	return nil
}

func (c *Collector) collectDaemonSetList(ctx context.Context) error {
	dsl, err := c.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.DaemonSetList = dsl
	return nil
}

func (c *Collector) collectStatefulSetList(ctx context.Context) error {
	stsl, err := c.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.StatefulSetList = stsl
	return nil
}

func (c *Collector) collectReplicationControllerList(ctx context.Context) error {
	rc, err := c.clientset.CoreV1().ReplicationControllers("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.ReplicationControllerList = rc
	return nil
}

func (c *Collector) collectServiceList(ctx context.Context) error {
	svc, err := c.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.ServiceList = svc
	return nil
}

func (c *Collector) collectCSINodeList(ctx context.Context) error {
	csin, err := c.clientset.StorageV1().CSINodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.CSINodeList = csin
	return nil
}

func (c *Collector) collectStorageClassList(ctx context.Context) error {
	scl, err := c.clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.StorageClassList = scl
	return nil
}

func (c *Collector) collectJobList(ctx context.Context) error {
	jobs, err := c.clientset.BatchV1().Jobs("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.JobList = jobs
	return nil
}
