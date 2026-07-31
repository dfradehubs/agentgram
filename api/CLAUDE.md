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

## Group Debates (moderated multi-agent)

`POST /api/groups/{groupId}/chat` runs a **moderated debate**: an LLM moderator
(LLM model with role `moderator`, wired like the summarizer) picks which agents
of the group speak, in sequence, each seeing prior turns via the existing
multi-agent context delta (`proxy.PrepareMessagesForMultiAgent`). Key pieces:

- **`orchestrator/`**: `Moderator.NextSpeaker` (LLM pick or FINISH) + `Debate`
  loop (cap `maxTurns`, failed turns don't abort) + optional `Synthesize`.
  Surface-agnostic via an injected `TurnRunner`.
- **SSE surface** (`handlers/group_chat.go`): one outer `RUN_STARTED` and one terminal
  `RUN_FINISHED` or `RUN_ERROR`; `debate.incomplete` reports partial outcomes;
  per-turn lifecycle suppressed via `HandleOptions.SuppressLifecycle`; all
  `TEXT_MESSAGE_*`/`TOOL_CALL_START` events tagged with `agentId` via
  `HandleOptions.AgentID`. Optional `agent_ids` in the body restricts the roster.
- **MCP surface** (`mcpserver/group.go`): tool `group__<groupId>` (synchronous,
  lower cap, progress notifications), collected replies as one markdown result.

## Administration tools on the MCP facade

`/mcp` also exposes the administration surface as 14 `admin_*` tools (agent and
MCP-server CRUD for editors; delete, audit, observability and global settings for
admins). Key pieces:

- **`mcpserver/admin.go`**: declarative `adminTools` table (`{name, method, path,
  minRole, schema, annotations}`). A tool call is **replayed in-process against the
  admin router** (`buildAdminRouter` in `server/routes_admin.go`, mounted at
  `/api/admin` and handed to the handler via `SetAdminRouter`), so validation,
  `RoleGate`, `registry.Refresh` and auditing stay in the handlers the web admin
  uses. Adding an operation is one row in the table.
- **Role filtering**: `tools/list` advertises only what `UserService.ResolveRole`
  allows, so ordinary users see no admin tools. The `RoleGate` on the router still
  enforces it on every call (a cached tool gets a 403).
- **Annotations**: `readOnlyHint` / `destructiveHint` / `idempotentHint` plus an
  explicit "do not call without the user asking for this specific change" warning
  in every description — the brake against a model reconfiguring the instance.
- **Secrets**: responses go through `security.RedactJSON` (`bearer_token`,
  `api_key`, `oauth2_client_secret`, all `headers` values → `"***"`). Because a
  read-edit-write round-trip would otherwise wipe a credential, a `PUT` carrying
  the sentinel triggers an internal `GET` and `security.RestoreRedacted` puts the
  stored values back.
- **Path safety**: `{id}` arguments must match `^[A-Za-z0-9._@+-]{1,128}$`, so an
  argument can't steer the loopback to another endpoint.

## Auditing admin operations

`middleware.AdminAudit` is mounted on the admin router, so **every** configuration
change is recorded in `audit_events` (the table behind `/api/admin/audit` and the
audit panel) whether it came from the web admin or the MCP tools, including
endpoints added later. Entries use `resource_type="admin"`,
`resource_id="<section>:<id>"`, `action=admin_create|admin_update|admin_delete`
and `source=web|mcp`. Denied attempts (403/400) are recorded too. The surface is
tagged via `middleware.WithSurfaceMCP` on the **context**, not a header, so an
external client can't forge it. Request/response bodies are redacted before
storage.

The pre-existing `audit_log` table (`auditRepo.Log` inside each admin handler) is
unchanged; it has no HTTP endpoint and is now a duplicate of the above.

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
- API (:8080) with `configs/config.dev.yaml` (auth disabled; includes a dev-only
  MCP static token `dev-token` so the MCP facade is testable without Keycloak)
- Mock agent (:9000) - supports REST SSE, A2A JSON-RPC, Sessions API, and an
  **OpenAI-compatible mock LLM** at `/v1/chat/completions` that plays the
  group-debate moderator deterministically
- Redis (:6379) - session storage + pub/sub
- PostgreSQL (:5432) - groups + persistent data
- Test frontend (:3001)

To test **group debates** end-to-end without a real LLM key, register the mock
as the moderator (LLM models support an optional custom `endpoint` for
OpenAI-compatible servers — Ollama, LiteLLM, gateways, mocks):

```bash
curl -X POST localhost:8080/api/admin/llm -H 'Content-Type: application/json' -d '{
  "id":"mock-moderator","name":"Mock Moderator","provider":"openai","model":"mock-gpt",
  "api_key":"mock-key","endpoint":"http://mock-agent:9000/v1/chat/completions",
  "role":"moderator","enabled":true}'
# restart the API (the moderator is wired at startup, like the summarizer)
```
