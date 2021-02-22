# CAST AI K8S Agent

This is non-prod ready **POC** (playing with exporting the relevant data)
- To make use of this, related code changes are done at the following projects, most of the logic will be there:
  - kubecast/telemetry
  - kubecast/autoscaler
  - console-ui

## Commands

### build docker image

```make build```

### build & push docker image

```make release```

### deploy to current K8S context

Deploys agent, to current K8S context, using environment variables as listed in `.env.example`

```make deploy```

