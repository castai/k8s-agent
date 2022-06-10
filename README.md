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

#### EKS

```text
PROVIDER=eks
EKS_ACCOUNT_ID=your-aws-account-id
EKS_REGION=your-cluster-region
EKS_CLUSTER_NAME=your-cluster-name
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

#### kOps

```text
PROVIDER=kops
```

#### Issues
If you encounter "Error: no Auth Provider found for name "gcp"", add a discard import to the main fn: 
```go
import (
    _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)
```

## Licence

[Apache 2.0 License](LICENSE)
