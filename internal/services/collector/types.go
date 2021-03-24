package collector

import (
	appv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
)

type ClusterData struct {
	NodeList                  *corev1.NodeList                  `json:"nodeList"`
	PodList                   *corev1.PodList                   `json:"podList"`
	PersistentVolumeList      *corev1.PersistentVolumeList      `json:"persistentVolumeList"`
	PersistentVolumeClaimList *corev1.PersistentVolumeClaimList `json:"persistentVolumeClaimList"`
	DeploymentList            *appv1.DeploymentList             `json:"deploymentList"`
	ReplicaSetList            *appv1.ReplicaSetList             `json:"replicaSetList"`
	DaemonSetList             *appv1.DaemonSetList              `json:"daemonSetList"`
	StatefulSetList           *appv1.StatefulSetList            `json:"statefulSetList"`
	ReplicationControllerList *corev1.ReplicationControllerList `json:"replicationControllerList"`
	ServiceList               *corev1.ServiceList               `json:"serviceList"`
	CSINodeList               *storagev1.CSINodeList            `json:"csiNodeList"`
	StorageClassList          *storagev1.StorageClassList       `json:"storageClassList"`
	JobList                   *batchv1.JobList                  `json:"jobList"`
}
