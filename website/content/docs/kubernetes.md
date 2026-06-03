---
title: Kubernetes
weight: 3
---

Agentgram deploys on Kubernetes with the [bjw-s `app-template`](https://github.com/bjw-s-labs/helm-charts)
chart, which renders plain Kubernetes objects (Deployment, Service, ConfigMap, HPA, PDB) — no
service mesh or vendor CRDs required.

Ready-to-use values live in [`examples/kubernetes/`](https://github.com/dfradehubs/agentgram/tree/main/examples/kubernetes).

## Prerequisites

- A cluster with `kubectl` / `helm` configured.
- **PostgreSQL** and **Redis** reachable in-cluster (e.g. the Bitnami charts). Adjust `POSTGRES_HOST`
  and `REDIS_ADDR` in `values-api.yaml` to match your services.
- An Ingress controller (the example uses `nginx`).

## 1. Add the chart repo

```bash
helm repo add bjw-s https://bjw-s-labs.github.io/helm-charts
helm repo update
```

## 2. Create secrets

```bash
kubectl create namespace agentgram

kubectl create secret generic agentgram-secrets -n agentgram \
  --from-literal=postgres-password='CHANGE_ME' \
  --from-literal=redis-password='CHANGE_ME'
```

## 3. Install

```bash
helm install agentgram-api bjw-s/app-template -f values-api.yaml -n agentgram
helm install agentgram-web bjw-s/app-template -f values-web.yaml -n agentgram
kubectl apply -f ingress.yaml -n agentgram
```

The API runs database migrations automatically on startup.

## Routing

`ingress.yaml` sends the MCP/OAuth paths (`/mcp`, `/.well-known/`, `/register`) straight to the API,
and everything else to the web app. The web app proxies `/api/*` and `/auth/*` to the API in-cluster
via `BACKEND_URL`. SSE buffering is disabled on the Ingress so streaming responses flush in real time.

{{< callout type="warning" >}}
The example ConfigMap ships with `auth.enabled: false`. Enable Keycloak before exposing the instance
publicly — see [Configuration](configuration).
{{< /callout >}}
