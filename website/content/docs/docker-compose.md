---
title: Docker Compose
weight: 2
---

The quickest way to self-host Agentgram on a single VM or server. It runs the API, the web app, Redis
and PostgreSQL using the published container images.

Ready-to-use files live in [`examples/docker-compose/`](https://github.com/dfradehubs/agentgram/tree/main/examples/docker-compose).

## 1. Get the files

```bash
git clone https://github.com/dfradehubs/agentgram.git
cd agentgram/examples/docker-compose
cp .env.example .env
```

Edit `.env` and set at least a real `POSTGRES_PASSWORD`. Pin `AGENTGRAM_VERSION` to a release tag
(e.g. `v0.1.0`) instead of `latest` for production.

## 2. Start the stack

```bash
docker compose up -d
```

The web UI is served on **http://localhost:3000** and proxies API/auth calls to the API container
internally (`BACKEND_URL`). Database migrations run automatically on API startup.

## Images

```text
ghcr.io/dfradehubs/agentgram-api:<version>
ghcr.io/dfradehubs/agentgram-web:<version>
```

## Configuration

The API reads `config.yaml` (mounted into the container) and resolves every secret from the
environment via `${ENV:VAR}`. The example ships with `auth.enabled: false`.

{{< callout type="warning" >}}
**Do not expose an unauthenticated instance to the public internet.** Enable Keycloak in
`config.yaml` and put the web app behind your identity provider before going live. See
[Configuration]({{< relref "configuration" >}}).
{{< /callout >}}
