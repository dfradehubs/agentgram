---
title: Docker Compose
weight: 2
---

The quickest way to self-host Agentgram on a single VM or server.

## Try it (one file)

```bash
curl -fsSL https://raw.githubusercontent.com/dfradehubs/agentgram/main/docker-compose.yaml -o docker-compose.yaml
docker compose up -d
```

Web UI: **http://localhost:3000**. A demo agent is registered on first boot.

## Production files

Ready-to-use files live in [`examples/docker-compose/`](https://github.com/dfradehubs/agentgram/tree/main/examples/docker-compose).

```bash
git clone https://github.com/dfradehubs/agentgram.git
cd agentgram/examples/docker-compose
cp .env.example .env
```

Edit `.env` and set at least a real `POSTGRES_PASSWORD`. Pin `AGENTGRAM_VERSION` to a release tag
instead of `latest` for production.

```bash
docker compose up -d
```

The web UI is served on **http://localhost:3000** and proxies API/auth calls to the API container
internally (`BACKEND_URL`). Database migrations run automatically on API startup.

## All-in-one image

`ghcr.io/dfradehubs/agentgram` runs the API and the web UI in one container. Redis and PostgreSQL
still sit beside it (or use laptop embedded stores). See
[`examples/all-in-one/`](https://github.com/dfradehubs/agentgram/tree/main/examples/all-in-one).

## Images

```text
ghcr.io/dfradehubs/agentgram:<version>
ghcr.io/dfradehubs/agentgram-api:<version>
ghcr.io/dfradehubs/agentgram-web:<version>
```

## Configuration

The API reads `config.yaml` (baked into the image, or mounted) and resolves every secret from the
environment via `${ENV:VAR}`. The example ships with `auth.enabled: false`.

{{< callout type="warning" >}}
**Do not expose an unauthenticated instance to the public internet.** Enable basic auth or OIDC in
`config.yaml` before going live. See [Configuration]({{< relref "configuration" >}}).
{{< /callout >}}
