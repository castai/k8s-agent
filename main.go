package main

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type TelemetryData struct {
	CustomerToken string `json:"customerToken"`
	ClusterProvider string `json:"clusterProvider"`
	ClusterName string `json:"clusterName"`
	ClusterVersion string `json:"clusterVersion"`
	ClusterRegion string `json:"clusterRegion"`
	NodeList *v1.NodeList `json:"nodeList"`
	PodList *v1.PodList `json:"podList"`
}

func sendTelemetry(log *logrus.Logger, t *TelemetryData) error {
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}

	request  := bytes.NewBuffer(b)
	req, err := http.NewRequest(
		"POST",
		os.Getenv("TELEMETRY_API_URL"),
		request,
	)

	if err != nil {
		return err
	}

	req.Header.Set("X-API-Key", os.Getenv("TELEMETRY_API_KEY"))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	_, err = client.Do(req)
	if err != nil {
		return err
	}
	log.Infof(
		"request[Cap=%d] with nodes[%d], pods[%d] sent",
		request.Cap(),
		len(t.NodeList.Items),
		len(t.PodList.Items))
	return nil
}

func main() {
	log := logrus.New()
	log.Info("starting the agent")
	ctx := context.Background()
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	if err != nil {
		panic(err)
	}

	const interval = 10 * time.Second
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
			ClusterName: clusterName,
			ClusterRegion: clusterRegion,
			CustomerToken: os.Getenv("CUSTOMER_TOKEN"),
			NodeList: nodes,
			PodList:  pods,
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
