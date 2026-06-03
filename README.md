<p align="center">
  <img src="docs/agentgram-logo.svg" alt="Agentgram" width="72" height="72" />
</p>

<h1 align="center">Agentgram</h1>

<p align="center">
  <strong>One chat. Every agent. Any protocol.</strong>
</p>

<p align="center">
  A single front door for all your AI agents — Agentgram speaks REST, A2A and ADK on the way in,<br/>
  and emits clean <a href="https://docs.ag-ui.com/">AG-UI</a> events on the way out, so the UI never has to care how an agent is built.
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License: MIT" /></a>
  <img src="https://img.shields.io/badge/Go-1.25-00ADD8.svg" alt="Go 1.25" />
  <img src="https://img.shields.io/badge/Next.js-16-000000.svg" alt="Next.js 16" />
  <img src="https://img.shields.io/badge/protocol-AG--UI-7C3AED.svg" alt="AG-UI" />
</p>

---

## Why

I kept ending up with a different chat window for every agent I built — one for the Kubernetes
agent, another for logs, another for metrics — each speaking its own protocol. Agentgram is the
single front door I wanted: **one chat UI, one API, every agent**, regardless of how each one is
wired underneath.

The API is a multiplexer. It handles auth, permissions and session storage, talks to each agent in
its native protocol, and converts everything to a uniform stream of AG-UI events. The web client
just renders that stream — it never needs to know whether an answer came from a REST endpoint, an
A2A peer or a Google ADK app.

## Features

- 🔌 **Protocol-agnostic** — Custom REST/SSE, [A2A](docs/AGENT_A2A_CONTRACT.md) (JSON-RPC) and [Google ADK](docs/AGENT_ADK_CONTRACT.md) agents behind one interface.
- 📡 **AG-UI native** — the API emits standard [AG-UI](https://docs.ag-ui.com/) SSE events (`RUN_STARTED`, `TEXT_MESSAGE_*`, `TOOL_CALL_*`, `RUN_FINISHED`).
- 🧵 **Sessions that persist** — conversation history per agent, stored in Redis and managed by the API (agents stay stateless).
- 👥 **Multi-agent chats** — talk to several agents in one thread and propagate context between them.
- 🔐 **Auth & permissions** — optional Keycloak (OIDC/JWT) login, with access controlled per agent by Google Workspace groups or individual users.
- 🛠️ **MCP server** — expose your agents as tools inside Claude Code and Cursor, with full OAuth + Dynamic Client Registration (no manual setup).
- 💬 **Slack integration** — reach the same agents from Slack.
- 📊 **Built-in observability** — usage metrics, latency and cost dashboards out of the box.
- 🧰 **Admin panel** — register agents, MCP servers, LLMs and permissions from the UI.

## Architecture

```
┌──────────────────────┐     ┌─────────────────────┐     ┌─────────────────┐
│        Web           │────>│        API          │────>│  Agent (Custom) │
│  (Next.js + AG-UI)   │<────│   (Go Multiplexer)  │<────│  REST · SSE     │
└──────────────────────┘     │                     │     └─────────────────┘
         ↑                   │  - JWT / OIDC auth  │     ┌─────────────────┐
    AG-UI events             │  - Permissions      │────>│  Agent (A2A)    │
    over SSE                 │  - Session store    │<────│  JSON-RPC       │
                             │  - AG-UI conversion │     └─────────────────┘
                             │  - MCP server       │     ┌─────────────────┐
                             └─────────────────────┘────>│  Agent (ADK)    │
                                       │            <────│  REST · SSE     │
                                  Redis · PostgreSQL     └─────────────────┘
```

See [docs/PROTOCOLS_OVERVIEW.md](docs/PROTOCOLS_OVERVIEW.md) for how each protocol is mapped onto AG-UI.

## Quick start

**Requirements:** Node.js 22+, Go 1.25+, Docker & Docker Compose.

```bash
git clone https://github.com/dfradehubs/agentgram.git
cd agentgram
make install        # install web + Go dependencies
make docker-up      # API + mock agent + Redis + PostgreSQL + a test frontend
```

That brings up:

| Service        | URL                     |
| -------------- | ----------------------- |
| API            | http://localhost:8080   |
| Mock agent     | http://localhost:9000   |
| Test frontend  | http://localhost:3001   |

To run the real web UI against the stack:

```bash
make web            # Next.js dev server on http://localhost:3000
```

Run `make help` to see every available target.

## How it works

A chat request is a list of messages plus an optional session id:

```http
POST /api/agents/{agentId}/chat
Content-Type: application/json
Authorization: Bearer <jwt>     # only when auth is enabled

{
  "messages": [
    { "role": "user", "content": "Hello" }
  ],
  "session_id": "optional-uuid"
}
```

The response is a stream of AG-UI events over SSE — the same shape no matter which protocol the agent speaks:

```
data: {"type":"RUN_STARTED","threadId":"...","runId":"..."}
data: {"type":"TEXT_MESSAGE_START","messageId":"...","role":"assistant"}
data: {"type":"TEXT_MESSAGE_CONTENT","messageId":"...","delta":"Hello"}
data: {"type":"TEXT_MESSAGE_END","messageId":"..."}
data: {"type":"RUN_FINISHED","threadId":"...","runId":"..."}
```

Full API reference: [docs/API.md](docs/API.md). Agent contracts: [REST](docs/AGENT_REST_CONTRACT.md) · [A2A](docs/AGENT_A2A_CONTRACT.md) · [ADK](docs/AGENT_ADK_CONTRACT.md).

## Deploying to the world

Ready-to-use deployment recipes live in [`examples/`](examples/):

- **[Docker Compose](examples/docker-compose/)** — self-host the whole stack (API + web + Redis + PostgreSQL) with the published images. The fastest way to put Agentgram on a server.
- **[Kubernetes (Helm)](examples/kubernetes/)** — production values for the [bjw-s `app-template`](https://github.com/bjw-s/helm-charts) chart, plus an Ingress that routes the web app and the MCP/OAuth endpoints correctly.

Container images are published to GitHub Container Registry on every release:

```
ghcr.io/dfradehubs/agentgram-api
ghcr.io/dfradehubs/agentgram-web
```

## Configuration

The API is configured from a single YAML file (`CONFIG_PATH`, e.g. `api/configs/config.yaml`) that
covers the server, auth, Redis, PostgreSQL, metrics, tracing and the MCP server. Secrets are never
written inline — every sensitive value uses `${ENV:VAR}` and is resolved from the environment:

```yaml
auth:
  enabled: true          # set false to run without login (local / trusted networks)
  keycloak:
    enabled: true
    issuer: "${ENV:KEYCLOAK_ISSUER}"
    client_id: "${ENV:OIDC_CLIENT_ID}"
    client_secret: "${ENV:OIDC_CLIENT_SECRET}"

redis:
  addr: "${ENV:REDIS_ADDR}"
  password: "${ENV:REDIS_PASSWORD}"
```

See [`api/.env.example`](api/.env.example) for the full list of variables.

**Agents are managed at runtime** from the built-in admin panel (`/admin`) and stored in PostgreSQL —
no redeploy needed to add one. For each agent you choose its protocol (Custom / A2A / ADK), its
endpoint, and who can reach it (Google Workspace groups or individual users).

## Claude Code & Cursor (MCP)

Agentgram exposes an MCP endpoint that turns your agents into tools inside Claude Code and Cursor. It
implements the full discovery + auth flow — protected resource metadata (RFC 9728), authorization
server discovery (RFC 8414) and **Dynamic Client Registration (RFC 7591)** — so there is nothing to
configure by hand:

```bash
claude mcp add --transport http agentgram https://agentgram.example.com/mcp
```

Then run `/mcp` in Claude Code, authenticate once, and ask away:

```
Ask logs-agent for the errors in the last 30 minutes of the payment-api service.
```

Full guide (Cursor, automations, tuning): [docs/MCP.md](docs/MCP.md).

## Tech stack

**API** — Go 1.25 · Chi router · Redis (sessions + pub/sub) · PostgreSQL · OpenTelemetry · structured logging.

**Web** — Next.js 16 (App Router) · TypeScript · Tailwind CSS 4 · SSE/AG-UI consumed directly via `fetch` + `ReadableStream` · Pino logging · bilingual UI (English / Spanish).

## Project structure

```
agentgram/
├── api/        # Go API (multiplexer) — proxy, agents, auth, mcp, store, repository
├── web/        # Next.js web client (AG-UI over SSE)
├── docs/       # Architecture, API and protocol contracts
├── examples/   # Docker Compose & Kubernetes deployment recipes
└── Makefile    # Development & build commands
```

Deeper dives: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) · [api/CLAUDE.md](api/CLAUDE.md) · [web/CLAUDE.md](web/CLAUDE.md).

## Contributing

Issues and pull requests are welcome. For anything non-trivial, open an issue first so we can talk it
through. Keep PRs focused, run `make test` and `make lint` before pushing, and follow
[Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`, …).

## License

Released under the [MIT License](LICENSE). © Daniel Fradejas.
