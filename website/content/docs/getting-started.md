---
title: Getting Started
weight: 1
---

The fastest way to see Agentgram running is the bundled Docker development environment. It starts the
API, a mock agent, Redis, PostgreSQL and a small test frontend.

## Requirements

- Node.js 22+
- Go 1.25+
- Docker & Docker Compose

## Clone and run

```bash
git clone https://github.com/dfradehubs/agentgram.git
cd agentgram
make install      # install web + Go dependencies
make docker-up    # API + mock agent + Redis + PostgreSQL + test frontend
```

This brings up:

| Service        | URL                     |
| -------------- | ----------------------- |
| API            | http://localhost:8080   |
| Mock agent     | http://localhost:9000   |
| Test frontend  | http://localhost:3001   |

To run the real web UI against the stack:

```bash
make web          # Next.js dev server on http://localhost:3000
```

Run `make help` to list every available target.

{{< callout type="info" >}}
The development environment runs with **authentication disabled** so you can try things immediately.
Don't expose it to the public internet — see [Configuration]({{< relref "configuration" >}}) to enable Keycloak.
{{< /callout >}}

## What next?

- Put it on a server with [Docker Compose]({{< relref "docker-compose" >}}) or [Kubernetes]({{< relref "kubernetes" >}}).
- Register your own agents — see [Agents & protocols]({{< relref "agents" >}}).
- Wire it into your IDE or CLI via [MCP]({{< relref "mcp" >}}).
