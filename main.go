package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/tools/clientcmd"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type TelemetryData struct {
	CustomerToken   string       `json:"customerToken"`
	ClusterProvider string       `json:"clusterProvider"`
	ClusterName     string       `json:"clusterName"`
	ClusterVersion  string       `json:"clusterVersion"`
	ClusterRegion   string       `json:"clusterRegion"`
	NodeList        *v1.NodeList `json:"nodeList"`
	PodList         *v1.PodList  `json:"podList"`
}

func sendTelemetry(log *logrus.Logger, t *TelemetryData) error {
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}

	request := bytes.NewBuffer(b)
	req, err := http.NewRequest(
		"POST",
		os.Getenv("API_URL"),
		request,
	)

	if err != nil {
		return err
	}

	req.Header.Set("X-API-Key", os.Getenv("API_KEY"))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	log.Infof(
		"request[Cap=%d] with nodes[%d], pods[%d] sent, responseCode=%v",
		request.Cap(),
		len(t.NodeList.Items),
		len(t.PodList.Items),
		resp.StatusCode)
	return nil
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

	const interval = 30 * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
		}

		nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Errorf("failed: %v", err)
			panic(err)
		}

		pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Errorf("failed: %v", err)
		}

		node1 := nodes.Items[0]
		clusterName := node1.Labels["alpha.eksctl.io/cluster-name"]
		clusterRegion := node1.Labels["topology.kubernetes.io/region"]

		t := &TelemetryData{
			ClusterProvider: "EKS",
			ClusterName:     clusterName,
			ClusterRegion:   clusterRegion,
			CustomerToken:   os.Getenv("CUSTOMER_TOKEN"),
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
		return nil, fmt.Errorf("KUBECONFIG not set")
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
	if kubeconfig != nil && err == nil {
		return kubeconfig, nil
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	return config, nil
}
