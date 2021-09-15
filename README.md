# CAST AI K8S Agent

## Local Development Setup

Agent can be tested locally using [Kind](https://kind.sigs.k8s.io) as a runtime environment.

```shell
$ brew install kind
```

```shell
kind create cluster --name cast-agent-test
kind get kubeconfig --name cast-agent-test > kind.kubeconfig
```



## Commands

### build docker image

```make build```

### build & push docker image

```make release```

### deploy to current K8S context

Deploys agent, to current K8S context, using environment variables as listed in `.env.example`

```make deploy```

