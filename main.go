package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	TIMEOUT = 10 * time.Second
)

type Request struct {
	Payload []byte `json:"payload"`
}

type TelemetryData struct {
	ClusterID       string       `json:"clusterId"`
	AccountID       string       `json:"accountId"`
	OrganizationID  string       `json:"organizationId"`
	ClusterProvider string       `json:"clusterProvider"`
	ClusterName     string       `json:"clusterName"`
	ClusterVersion  string       `json:"clusterVersion"`
	ClusterRegion   string       `json:"clusterRegion"`
	NodeList        *v1.NodeList `json:"nodeList"`
	PodList         *v1.PodList  `json:"podList"`
}

type EKSParams struct {
	ClusterName    string `json:"cluster_name"`
	OrganizationID string `json:"organization_id"`
	Region         string `json:"region"`
	AccountID      string `json:"account_id"`
}

type RegisterClusterRequest struct {
	Name           string    `json:"name"`
	OrganizationID string    `json:"organization_id"`
	EKS            EKSParams `json:"eks"`
}

type Cluster struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	OrganizationID string    `json:"organization_id"`
	EKS            EKSParams `json:"eks"`
}

type RegisterClusterResponse struct {
	Cluster Cluster `json:"cluster"`
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

	c, err := registerCluster(log, &RegisterClusterRequest{
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
		}

		nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Errorf("failed: %v", err)
		}

		pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Errorf("failed: %v", err)
		}

		t := &TelemetryData{
			OrganizationID:  c.Cluster.OrganizationID,
			ClusterID:       c.Cluster.ID,
			AccountID:       awsAccountId,
			ClusterProvider: "EKS",
			ClusterName:     clusterName,
			ClusterRegion:   clusterRegion,
			NodeList:        nodes,
			PodList:         pods,
		}

		version, err := clientset.ServerVersion()
		if err != nil {
			log.Errorf("failed to get cluster version: %v", version)
		}

		t.ClusterVersion = version.GitVersion

		err = sendTelemetry(log, t)

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

func registerCluster(log *logrus.Logger, registerRequest *RegisterClusterRequest) (*RegisterClusterResponse, error) {
	r, err := json.Marshal(registerRequest)
	if err != nil {
		return nil, err
	}

	request := bytes.NewBuffer(r)
	req, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("https://%s/v1/kubernetes/external-clusters", os.Getenv("API_URL")),
		request,
	)

	if err != nil {
		return nil, err
	}

	req.Header.Set("X-API-Key", os.Getenv("API_KEY"))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: TIMEOUT}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var dest RegisterClusterResponse
	if err := json.NewDecoder(resp.Body).Decode(&dest); err != nil {
		return nil, err
	}

	log.Infof("cluster[%s] registered with id: %s", dest.Cluster.Name, dest.Cluster.ID)
	return &dest, nil
}

func sendTelemetry(log *logrus.Logger, t *TelemetryData) error {
	tb, err := json.Marshal(t)
	if err != nil {
		return err
	}

	b, err := json.Marshal(&Request{Payload: tb})
	if err != nil {
		return err
	}

	request := bytes.NewBuffer(b)
	req, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("https://%s/v1/agent/eks-snapshot", os.Getenv("API_URL")),
		request,
	)

	if err != nil {
		return err
	}

	req.Header.Set("X-API-Key", os.Getenv("API_KEY"))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: TIMEOUT}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	log.Infof(
		"request[Cap=%d] with nodes[%d], pods[%d] sent, responseCode=%v",
		request.Cap(),
		len(t.NodeList.Items),
		len(t.PodList.Items),
		resp.StatusCode)
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
