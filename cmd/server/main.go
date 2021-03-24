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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"

	"castai-agent/internal/services/collector"

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

type TelemetrySnapshot struct {
	ClusterID       string `json:"clusterId"`
	AccountID       string `json:"accountId"`
	OrganizationID  string `json:"organizationId"`
	ClusterProvider string `json:"clusterProvider"`
	ClusterName     string `json:"clusterName"`
	ClusterVersion  string `json:"clusterVersion"`
	ClusterRegion   string `json:"clusterRegion"`
	*collector.ClusterData
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

	col := collector.NewCollector(clientset)

	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}

		cd, err := col.Collect(ctx)
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
