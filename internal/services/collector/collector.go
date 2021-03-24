package collector

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type collector struct {
	clientset *kubernetes.Clientset
	cd        *ClusterData
}

func NewCollector(clientset *kubernetes.Clientset) *collector {
	var cd ClusterData
	return &collector{
		clientset: clientset,
		cd:        &cd,
	}
}

func (c *collector) Collect(ctx context.Context) (*ClusterData, error) {
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

func (c *collector) collectNodes(ctx context.Context) error {
	nodes, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.NodeList = nodes
	return nil
}

func (c *collector) collectPods(ctx context.Context) error {
	pods, err := c.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.PodList = pods
	return nil
}

func (c *collector) collectPersistentVolumes(ctx context.Context) error {
	pods, err := c.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.PodList = pods
	return nil
}

func (c *collector) collectPersistentVolumeClaims(ctx context.Context) error {
	pvc, err := c.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.PersistentVolumeClaimList = pvc
	return nil
}

func (c *collector) collectDeploymentList(ctx context.Context) error {
	dpls, err := c.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.DeploymentList = dpls
	return nil
}

func (c *collector) collectReplicaSetList(ctx context.Context) error {
	rpsl, err := c.clientset.AppsV1().ReplicaSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.ReplicaSetList = rpsl
	return nil
}

func (c *collector) collectDaemonSetList(ctx context.Context) error {
	dsl, err := c.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.DaemonSetList = dsl
	return nil
}

func (c *collector) collectStatefulSetList(ctx context.Context) error {
	stsl, err := c.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.StatefulSetList = stsl
	return nil
}

func (c *collector) collectReplicationControllerList(ctx context.Context) error {
	rc, err := c.clientset.CoreV1().ReplicationControllers("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.ReplicationControllerList = rc
	return nil
}

func (c *collector) collectServiceList(ctx context.Context) error {
	svc, err := c.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.ServiceList = svc
	return nil
}

func (c *collector) collectCSINodeList(ctx context.Context) error {
	csin, err := c.clientset.StorageV1().CSINodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.CSINodeList = csin
	return nil
}

func (c *collector) collectStorageClassList(ctx context.Context) error {
	scl, err := c.clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.StorageClassList = scl
	return nil
}

func (c *collector) collectJobList(ctx context.Context) error {
	jobs, err := c.clientset.BatchV1().Jobs("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	c.cd.JobList = jobs
	return nil
}
