# CAST AI Kubernetes Agent

A component that connects your Kubernetes cluster to the [CAST AI](https://www.cast.ai) platform to enable Kubernetes automation and cost optimization features.

## Getting started

Visit the [docs](https://docs.cast.ai/docs/getting-started) to connect your cluster.

## Helm chart

The helm chart for the CAST AI Kubernetes agent is published in the [castai/helm-charts](https://github.com/castai/helm-charts) repo.


## Reading configuration from file

You can pass configuration to agent via YAML file.

Example file
```yaml
api:
  key: "api key"
  url: "api.cast.ai"
```
or

```yaml
api.key: "api key"
api.url: "api.cast.ai"
```

Shell example

```shell 
CONFIG_PATH=<PATH_TO_CONFIG> ./castai-agent
```

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

If you want to test changes with existing cluster already onboarded to CAST AI console, you can set environment variable 

```text
STATIC_CLUSTER_ID=your-cluster-id
```

#### Issues

If you encounter "Error: no Auth Provider found for name "gcp"", add a discard import to the main fn: 
```go
import (
    _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)
```

### Test agent in a containerized/packaged environment

* Prepare a repository to work with your cluster, e.g. Google Artifact Repository if working with a GKE cluster
* Create and push agent image with `TAG=path/to/repository/k8s-agent:1.1-your-test-tag make build`. This will build the multi-architecture agent docker image and push it to the repository of your choice. `TAG` in this case should refer to the full image path and will be used for `docker push` command as-is. Increment the version when building more test versions.
* Clone [castai/helm-charts](https://github.com/castai/helm-charts) repo;
* Add necessary changes to the chart (if needed)
* Clone values.yaml and tweak these chart values:
  * `image.repository`: for image build above, set this to `path/to/repository/k8s-agent`
  * `image.tag`: for above example, this would be `1.1-your-test-tag`
  * `clusterVPA.enabled`: turn off for testing setups
  * `apiURL`: override if using non-production CAST AI environment (CAST AI developers only)
  * `apiKey`: API key to use for the agent
* Deploy the chart:
  ```
  helm template castai-agent . -n castai-agent -f values.ignore.yaml | kubectl apply -f -
  ```
  

### Release procedure (with automatic release notes)

Head to the [GitHub new release page](https://github.com/castai/k8s-agent/releases/new), create a new tag at the top, and click `Generate Release Notes` at the middle-right.
![image](https://user-images.githubusercontent.com/571022/174777789-2d7d646d-714d-42da-8c66-a6ed407b4440.png)


## Licence

[Apache 2.0 License](LICENSE)
