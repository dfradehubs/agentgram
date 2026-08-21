---
title: Getting Started
weight: 1
---

The fastest way to see Agentgram is a single Docker Compose file. It starts the API, the web UI,
Redis and PostgreSQL. A **built-in demo agent** is registered on first boot, so you can chat without
opening the admin panel.

## Requirements

- Docker (Compose v2)

## Run it

```bash
curl -fsSL https://raw.githubusercontent.com/dfradehubs/agentgram/main/docker-compose.yaml -o docker-compose.yaml
docker compose up -d
```

| Service | URL |
| ------- | --- |
| Web UI | http://localhost:3000 |
| API / MCP | http://localhost:8080 |
| MCP endpoint | http://localhost:8080/mcp |

Open the UI and send a message to **Demo**. Then register your own agents from Admin → Agents, or from
the first-run wizard if the list is empty.

Connect an IDE:

```bash
claude mcp add --transport http agentgram http://localhost:8080/mcp
```

{{< callout type="info" >}}
This stack runs with **authentication disabled**. Don't expose it to the public internet.
Use [basic username/password]({{< relref "configuration" >}}) or generic OIDC before going live.
{{< /callout >}}

## Self-host from the repo

```bash
git clone https://github.com/dfradehubs/agentgram.git
cd agentgram/examples/docker-compose
cp .env.example .env
docker compose up -d
```

Pin `AGENTGRAM_VERSION` to a release tag in production. A single image
`ghcr.io/dfradehubs/agentgram` runs API + UI in one container (Redis and Postgres still sit beside it).

## Laptop mode (no Docker for datastores)

From a Go checkout:

```bash
cd api
CONFIG_PATH=configs/config.laptop.yaml go run ./cmd/server
```

That starts **embedded Redis** and **embedded PostgreSQL** in-process. Point the web app at
`http://localhost:8080` or set `WEB_STATIC_DIR` if you are serving a static UI from the API.

## Develop from source

- Node.js 22+
- Go 1.25+
- Docker

```bash
git clone https://github.com/dfradehubs/agentgram.git
cd agentgram
make install
make docker-up
```

`make docker-up` also starts a mock REST agent on `:9000` (seeded automatically when
`MOCK_REST_ENDPOINT` is set).

## What next?

- Put it on a server with [Docker Compose]({{< relref "docker-compose" >}}) or [Kubernetes]({{< relref "kubernetes" >}}).
- Register your own agents — see [Agents & protocols]({{< relref "agents" >}}).
- Wire it into your IDE or CLI via [MCP]({{< relref "mcp" >}}).
- How Agentgram differs from Open WebUI / MCPJungle / ContextForge: [Compare]({{< relref "compare" >}}).
