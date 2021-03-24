package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
	appv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	defaultRetryCount = 3
	defaultTimeout    = 10 * time.Second
)

func NewDefaultClient() *resty.Client {
	client := resty.New()
	client.SetRetryCount(defaultRetryCount)
	client.SetTimeout(defaultTimeout)
	client.Header.Set("X-API-Key", os.Getenv("API_KEY"))
	client.Header.Set("Content-Type", "application/json")
	return client
}

type Request struct {
	Payload []byte `json:"payload"`
}

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

type TelemetrySnapshot struct {
	ClusterID       string `json:"clusterId"`
	AccountID       string `json:"accountId"`
	OrganizationID  string `json:"organizationId"`
	ClusterProvider string `json:"clusterProvider"`
	ClusterName     string `json:"clusterName"`
	ClusterVersion  string `json:"clusterVersion"`
	ClusterRegion   string `json:"clusterRegion"`
	*ClusterData
}

type EKSParams struct {
	ClusterName string `json:"clusterName"`
	Region      string `json:"region"`
	AccountID   string `json:"accountId"`
}

type RegisterClusterRequest struct {
	Name string    `json:"name"`
	EKS  EKSParams `json:"eks"`
}

type Cluster struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	OrganizationID string    `json:"organizationId"`
	EKS            EKSParams `json:"eks"`
}

type RegisterClusterResponse struct {
	Cluster
}

func main() {
	log := logrus.New()
	log.Info("starting the agent")
	ctx := context.Background()
	config, err := retrieveKubeConfig()
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	restclient := NewDefaultClient()

	const interval = 15 * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	awsAccountId, err := retrieveAwsAccountId()
	if err != nil {
		panic(err)
	}

	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Errorf("failed: %v", err)
		panic(err)
	}

	node1 := nodes.Items[0]
	clusterName := node1.Labels["alpha.eksctl.io/cluster-name"]
	clusterRegion := node1.Labels["topology.kubernetes.io/region"]

	c, err := registerCluster(log, restclient, &RegisterClusterRequest{
		Name: clusterName,
		EKS: EKSParams{
			AccountID:   awsAccountId,
			Region:      clusterRegion,
			ClusterName: clusterName,
		},
	})

	if err != nil {
		panic(err)
	}

	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}

		cd, err := collectAll(ctx, clientset)
		if err != nil {
			log.Errorf("failed collecting snapshot data: %v", err)
			continue
		}

		t := &TelemetrySnapshot{
			OrganizationID:  c.Cluster.OrganizationID,
			ClusterID:       c.Cluster.ID,
			AccountID:       awsAccountId,
			ClusterProvider: "EKS",
			ClusterName:     clusterName,
			ClusterRegion:   clusterRegion,
			ClusterData:     cd,
		}

		version, err := clientset.ServerVersion()
		if err != nil {
			log.Errorf("failed to get cluster version: %v", version)
		}

		t.ClusterVersion = version.GitVersion
		err = sendTelemetry(log, restclient, t)

		if err != nil {
			log.Errorf("failed to send data: %v", err)
		}
	}
}

func collectNodes(ctx context.Context, c *kubernetes.Clientset, cd *ClusterData) error {
	nodes, err := c.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	cd.NodeList = nodes
	return nil
}

func collectPods(ctx context.Context, c *kubernetes.Clientset, cd *ClusterData) error {
	pods, err := c.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	cd.PodList = pods
	return nil
}

func collectPersistentVolumes(ctx context.Context, c *kubernetes.Clientset, cd *ClusterData) error {
	pods, err := c.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	cd.PodList = pods
	return nil
}

func collectPersistentVolumeClaims(ctx context.Context, c *kubernetes.Clientset, cd *ClusterData) error {
	pvc, err := c.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	cd.PersistentVolumeClaimList = pvc
	return nil
}

func collectDeploymentList(ctx context.Context, c *kubernetes.Clientset, cd *ClusterData) error {
	dpls, err := c.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	cd.DeploymentList = dpls
	return nil
}

func collectReplicaSetList(ctx context.Context, c *kubernetes.Clientset, cd *ClusterData) error {
	rpsl, err := c.AppsV1().ReplicaSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	cd.ReplicaSetList = rpsl
	return nil
}

func collectDaemonSetList(ctx context.Context, c *kubernetes.Clientset, cd *ClusterData) error {
	dsl, err := c.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	cd.DaemonSetList = dsl
	return nil
}

func collectStatefulSetList(ctx context.Context, c *kubernetes.Clientset, cd *ClusterData) error {
	stsl, err := c.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	cd.StatefulSetList = stsl
	return nil
}

func collectReplicationControllerList(ctx context.Context, c *kubernetes.Clientset, cd *ClusterData) error {
	rc, err := c.CoreV1().ReplicationControllers("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	cd.ReplicationControllerList = rc
	return nil
}

func collectServiceList(ctx context.Context, c *kubernetes.Clientset, cd *ClusterData) error {
	svc, err := c.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	cd.ServiceList = svc
	return nil
}

func collectCSINodeList(ctx context.Context, c *kubernetes.Clientset, cd *ClusterData) error {
	csin, err := c.StorageV1().CSINodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	cd.CSINodeList = csin
	return nil
}

func collectStorageClassList(ctx context.Context, c *kubernetes.Clientset, cd *ClusterData) error {
	scl, err := c.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	cd.StorageClassList = scl
	return nil
}

func collectJobList(ctx context.Context, c *kubernetes.Clientset, cd *ClusterData) error {
	jobs, err := c.BatchV1().Jobs("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	cd.JobList = jobs
	return nil
}

func collectAll(ctx context.Context, c *kubernetes.Clientset) (*ClusterData, error) {
	var cd ClusterData

	if err := collectNodes(ctx, c, &cd); err != nil {
		return nil, err
	}

	if err := collectPods(ctx, c, &cd); err != nil {
		return nil, err
	}

	if err := collectPods(ctx, c, &cd); err != nil {
		return nil, err
	}

	if err := collectPersistentVolumes(ctx, c, &cd); err != nil {
		return nil, err
	}

	if err := collectPersistentVolumeClaims(ctx, c, &cd); err != nil {
		return nil, err
	}

	if err := collectDeploymentList(ctx, c, &cd); err != nil {
		return nil, err
	}

	if err := collectReplicaSetList(ctx, c, &cd); err != nil {
		return nil, err
	}

	if err := collectDaemonSetList(ctx, c, &cd); err != nil {
		return nil, err
	}

	if err := collectStatefulSetList(ctx, c, &cd); err != nil {
		return nil, err
	}

	if err := collectReplicationControllerList(ctx, c, &cd); err != nil {
		return nil, err
	}

	if err := collectServiceList(ctx, c, &cd); err != nil {
		return nil, err
	}

	if err := collectCSINodeList(ctx, c, &cd); err != nil {
		return nil, err
	}

	if err := collectStorageClassList(ctx, c, &cd); err != nil {
		return nil, err
	}

	if err := collectJobList(ctx, c, &cd); err != nil {
		return nil, err
	}

	return &cd, nil
}

func kubeConfigFromEnv() (*rest.Config, error) {
	kubepath := os.Getenv("KUBECONFIG")
	if kubepath == "" {
		return nil, nil
	}

	data, err := ioutil.ReadFile(kubepath)
	if err != nil {
		return nil, fmt.Errorf("reading KUBECONFIG: %w", err)
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(data)
	if err != nil {
		return nil, fmt.Errorf("building REST config from KUBECONFIG: %w", err)
	}

	return restConfig, nil
}

func retrieveKubeConfig() (*rest.Config, error) {
	kubeconfig, err := kubeConfigFromEnv()
	if err != nil {
		return nil, err
	}

	if kubeconfig != nil {
		return kubeconfig, nil
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	return config, nil
}

func registerCluster(log *logrus.Logger, client *resty.Client, registerRequest *RegisterClusterRequest) (*RegisterClusterResponse, error) {
	resp, err := client.R().
		SetBody(registerRequest).
		SetResult(&RegisterClusterResponse{}).
		Post(fmt.Sprintf("https://%s/v1/kubernetes/external-clusters", os.Getenv("API_URL")))

	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to register cluster with StatusCode[%d]", resp.StatusCode())
	}

	log.Infof("cluster registered: %+v", resp.Result())
	return resp.Result().(*RegisterClusterResponse), nil
}

func sendTelemetry(log *logrus.Logger, client *resty.Client, t *TelemetrySnapshot) error {
	tb, err := json.Marshal(t)
	if err != nil {
		return err
	}

	resp, err := client.R().
		SetBody(&Request{Payload: tb}).
		SetResult(&RegisterClusterResponse{}).
		Post(fmt.Sprintf("https://%s/v1/agent/eks-snapshot", os.Getenv("API_URL")))

	if err != nil {
		return err
	}

	if resp.IsError() {
		return fmt.Errorf("failed to send snapshot with StatusCode[%d]", resp.StatusCode())
	}

	log.Infof(
		"request with nodes[%d], pods[%d] sent, responseCode=%d",
		len(t.NodeList.Items),
		len(t.PodList.Items),
		resp.StatusCode())
	return nil
}

func retrieveAwsAccountId() (string, error) {
	if accountId := os.Getenv("AWS_ACCOUNT_ID"); accountId != "" {
		return accountId, nil
	}

	s, err := session.NewSession()
	if err != nil {
		return "", fmt.Errorf("could not create AWS SDK session: %w", err)
	}

	meta := ec2metadata.New(s)
	doc, err := meta.GetInstanceIdentityDocument()
	if err != nil {
		return "", fmt.Errorf("failed to get instance identity document: %w", err)
	}

	return doc.AccountID, nil
}
