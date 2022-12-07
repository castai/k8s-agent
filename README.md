# CAST AI Kubernetes Agent

A component that connects your Kubernetes cluster to the [CAST AI](https://www.cast.ai) platform to enable Kubernetes automation and cost optimization features.

## Getting started

Visit the [docs](https://docs.cast.ai/getting-started/overview/) to connect your cluster.

## Helm chart

The helm chart for the CAST AI Kubernetes agent is published in the [castai/helm-charts](https://github.com/castai/helm-charts) repo.

## Contributing

### Run the agent in your IDE 

You must provide the these environment variables:

```text
API_KEY=your-castai-api-key
API_URL=api.cast.ai
KUBECONFIG=/path/to/kubeconfig
```

Then, based on the Kubernetes provider, you need to provide additional environment variables.

#### AKS

```text
PROVIDER=aks
AKS_LOCATION=your-cluster-location
AKS_SUBSCRIPTION_ID=your-cluster-subscription-id
AKS_NODE_RESOURCE_GROUP=your-cluster-resource-group
```

#### EKS

```text
PROVIDER=eks
EKS_ACCOUNT_ID=your-aws-account-id
EKS_REGION=your-cluster-region
EKS_CLUSTER_NAME=your-cluster-name
```

#### kOps

```text
PROVIDER=kops
```

#### GKE

```text
PROVIDER=gke
GKE_PROJECT_ID=your-gke-project-id
GKE_CLUSTER_NAME=your-cluster-name
GKE_REGION=your-cluster-region
GKE_LOCATION=your-cluster-location
```
note, when using zonal `GKE_REGION` and `GKE_LOCATION` is often the same, i.e. `europe-west3-a`


#### Issues

If you encounter "Error: no Auth Provider found for name "gcp"", add a discard import to the main fn: 
```go
import (
    _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)
```

### Release procedure (with automatic release notes)

Head to the [GitHub new release page](https://github.com/castai/k8s-agent/releases/new), create a new tag at the top, and click `Generate Release Notes` at the middle-right.
![image](https://user-images.githubusercontent.com/571022/174777789-2d7d646d-714d-42da-8c66-a6ed407b4440.png)


## Licence

[Apache 2.0 License](LICENSE)
