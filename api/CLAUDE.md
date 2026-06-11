# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

## Project Overview

Go API that acts as a multiplexer to connect to multiple remote agents. Manages JWT authentication (Keycloak), permissions (Google Workspace groups/users), and request forwarding. **All responses to the web use SSE with AG-UI protocol** regardless of agent protocol.

## Common Commands

```bash
# Build
go build ./...

# Run (development)
make dev                    # Direct run
make dev-watch              # With hot reload (requires air)

# Tests
make test                   # Unit tests
go test ./internal/agents/... -v -run TestPermissions  # Single test
make test-coverage          # Coverage report

# Lint
make lint
make lint-fix

# Docker (full environment with mock agents)
cd docker && docker compose up -d
cd docker && docker compose logs -f api
```

## Architecture

```
Web ──> API (Proxy) ──> Agent (Custom) → SSE/JSON → AG-UI
                    ──> Agent (A2A)  → Polling → AG-UI
```

### Key Flow
1. Request → Middleware (security headers, body limit, logging, auth, rate limit) → Handler
2. Handler → Registry (get agent config) → Permissions check
3. Proxy routes by protocol → Converts to AG-UI events for web

### Internal Packages

- **`proxy/`**: Main multiplexer. `proxy.go` routes to `rest.go` (custom), `a2a.go`, or `adk.go`. All use `sse.go` for AG-UI output.
- **`agents/`**: Agent registry, permissions logic (`HasAccess` with wildcard `*` support), health checker, Custom/A2A/ADK clients.
- **`auth/`**: JWT validation with JWKS caching from Keycloak. Validates issuer, audience (clientID), and nonce (OIDC flow).
- **`middleware/`**: Auth (with `NoAuth` for disabled auth), security headers, body limit, rate limiting, logging. The `responseWriter` wrapper implements `http.Flusher` for SSE.
- **`config/`**: YAML loading with `${ENV:VAR}` syntax for env substitution.
- **`handlers/`**: HTTP handlers including `sessions.go` for session CRUD (reads/writes directly to `store.SessionStore`).
- **`store/`**: Redis session persistence (`SessionStore` for session and message storage).
- **`repository/`**: PostgreSQL data access for groups and persistent entities.
- **`service/`**: Business logic layer.
- **`pubsub/`**: Real-time event distribution via Redis Pub/Sub.
- **`models/`**: Data types including `agui.go` for AG-UI events and `session.go` for session types.

### Outbound Headers to Agents and MCP Servers

Every agent request (REST/A2A/ADK clients in `agents/`) carries:

- `X-Agent-ID` — always (agents scope sessions by it)
- `X-Request-ID` — when present (end-to-end tracing)
- `X-User-Email` / `X-User-Groups` — calling user's identity from validated JWT/session claims in context (`identity.SetHeaders`). Unsigned assertions: agents must trust them only when the caller is provably agentgram. Groups are CSV, capped at 4KB
- `X-GitHub-Token` — only if agent has `require_github_token: true`
- Credential per `auth_type` (`agents.ResolveOutboundAuth`): `forward` → user's `Authorization` as-is; `bearer` → resolved API key; `none` → nothing
- Admin-configured agent headers, filtered by `security.FilterHeaders` (SSRF)

MCP server requests (`mcp.Client`) carry the same `X-User-Email` / `X-User-Groups`: tool calls get them from the request context at the client level; the `initialize` handshake (background context) gets them merged into the credential headers via `identity.Merge` in `resolveExtraHeaders` (web) and `handleMCPToolCall` (MCP facade).

## Configuration

Single YAML file loaded via env var:
- `CONFIG_PATH` → `configs/config.yaml` (server, auth, logging, cors, agents)

Auth can be disabled: `auth.enabled: false` in config.yaml (uses `NoAuth` middleware).

## Security

### Middleware Stack (order matters)
1. `SecurityHeaders` - X-Content-Type-Options, X-Frame-Options, HSTS, Referrer-Policy, Permissions-Policy
2. `BodyLimit(1MB)` - Prevents oversized request bodies
3. `Auth` - JWT (bearer) or OIDC session (cookie) validation with audience check
4. `RateLimiter.ChatHandler` - Per-user per-agent rate limiting (60 req/min) on chat endpoints

### Auth Security
- JWT validates: signature (JWKS), issuer, audience (clientID), expiration
- OIDC flow validates: state (CSRF), nonce (token substitution), audience
- Agent errors are sanitized before sending to clients (no internal details leaked)

### Configuration
- `POSTGRES_SSLMODE` env var controls DB SSL (default: `require`, dev: `disable`)
- `LOG_LEVEL` env var controls log verbosity (default: `info`, dev: `debug`)
- Secrets use `${ENV:VAR}` syntax - never hardcoded in YAML

## AG-UI SSE Response Format

The API emits AG-UI protocol events via SSE:

```
data: {"type":"RUN_STARTED","threadId":"...","runId":"..."}
data: {"type":"TEXT_MESSAGE_START","messageId":"...","role":"assistant"}
data: {"type":"TEXT_MESSAGE_CONTENT","messageId":"...","delta":"Hello"}
data: {"type":"TEXT_MESSAGE_END","messageId":"..."}
data: {"type":"RUN_FINISHED","threadId":"...","runId":"..."}
```

Event types:
- `RUN_STARTED` / `RUN_FINISHED` - Run lifecycle
- `RUN_ERROR` - Error during run
- `TEXT_MESSAGE_START` / `TEXT_MESSAGE_CONTENT` / `TEXT_MESSAGE_END` - Streaming text
- `TOOL_CALL_START` / `TOOL_CALL_ARGS` / `TOOL_CALL_END` - Tool call lifecycle
- `CUSTOM` - Custom event passthrough

## Sessions API

Sessions are managed by the API and stored in Redis via `store.SessionStore`. The `SessionsHandler` in `handlers/sessions.go` reads/writes directly to Redis:

```
GET    /api/agents/{agentId}/sessions              # List sessions from Redis
GET    /api/agents/{agentId}/sessions/{sessionId}  # Get session with messages from Redis
PATCH  /api/agents/{agentId}/sessions/{sessionId}  # Rename session in Redis
DELETE /api/agents/{agentId}/sessions/{sessionId}  # Delete session from Redis
```

## Test Environment

`docker/docker-compose.yml` runs:
- API (:8080) with `configs/config.dev.yaml` (auth disabled)
- Mock agent (:9000) - supports REST SSE, A2A JSON-RPC, and Sessions API
- Redis (:6379) - session storage + pub/sub
- PostgreSQL (:5432) - groups + persistent data
- Test frontend (:3001)
