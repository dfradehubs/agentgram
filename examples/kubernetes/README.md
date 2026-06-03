# Kubernetes deployment (Helm + bjw-s app-template)

These values deploy Agentgram on top of the [bjw-s `app-template`](https://github.com/bjw-s-labs/helm-charts)
chart, which renders plain Kubernetes objects (Deployment, Service, ConfigMap, HPA, PDB). No
service-mesh or vendor CRDs required.

## Prerequisites

- A Kubernetes cluster and `kubectl` / `helm` configured.
- **PostgreSQL** and **Redis** reachable in-cluster. The values assume services named
  `postgresql` and `redis-master` (e.g. the Bitnami charts) — adjust `POSTGRES_HOST` / `REDIS_ADDR`
  in `values-api.yaml` to match yours.
- An Ingress controller (the example uses `nginx`).

## 1. Add the chart repo

```bash
helm repo add bjw-s https://bjw-s-labs.github.io/helm-charts
helm repo update
```

> Check the chart's README for the current repo URL and latest `app-template` version.

## 2. Create the secrets

```bash
kubectl create namespace agentgram

kubectl create secret generic agentgram-secrets -n agentgram \
  --from-literal=postgres-password='CHANGE_ME' \
  --from-literal=redis-password='CHANGE_ME'
  # add oidc-client-id / oidc-client-secret here when you enable auth
```

## 3. Install

```bash
helm install agentgram-api bjw-s/app-template -f values-api.yaml -n agentgram
helm install agentgram-web bjw-s/app-template -f values-web.yaml -n agentgram
kubectl apply -f ingress.yaml -n agentgram
```

The API runs database migrations automatically on startup.

## 4. Verify

```bash
kubectl get pods -n agentgram
kubectl logs -n agentgram deploy/agentgram-api
```

Then browse to your Ingress host (e.g. `https://agentgram.example.com`).

## Routing

`ingress.yaml` sends the MCP/OAuth paths (`/mcp`, `/.well-known/`, `/register`) straight to the API,
and everything else to the web app. The web app proxies `/api/*` and `/auth/*` to the API in-cluster
via `BACKEND_URL`, so those don't need their own Ingress rules. SSE buffering is disabled on the
Ingress so streaming responses flush in real time.

## Before going public

The example ConfigMap ships with `auth.enabled: false`. Enable Keycloak (OIDC) in
`values-api.yaml`'s ConfigMap, add the OIDC secret keys, and put the app behind your identity
provider before exposing it. See [`../../api/docs/SECURITY.md`](../../api/docs/SECURITY.md).
