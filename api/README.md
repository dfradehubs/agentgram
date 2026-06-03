# Agentgram API

Go API that acts as a multiplexer to connect multiple AI agents. Manages JWT authentication (Keycloak), permissions by groups/users, and translates responses to the AG-UI protocol.

## Features

- **Agent multiplexer**: Connects to REST and A2A agents from a single endpoint
- **AG-UI protocol**: Emits AG-UI compatible events via SSE
- **Session management**: Redis-backed session store with atomic message deduplication
- **Authentication**: JWT validation with JWKS from Keycloak + OIDC session flow
- **Permissions**: Control by Google Workspace groups and individual users
- **Health checks**: Agent health monitoring
- **MCP support**: Model Context Protocol server integration

## Tech Stack

- **Go 1.25** - Main language
- **Chi** - Lightweight HTTP router
- **Zap** - Structured high-performance logging
- **Redis** - Session storage + Pub/Sub
- **PostgreSQL** - Persistent data (agents, groups, users)
- **YAML** - Configuration with environment variable support

## Quick Start

### With Docker (Recommended)

```bash
# Start API + mock agent + Redis + PostgreSQL
cd docker && docker compose up -d

# View logs
docker compose logs -f api
```

Services:
- API: http://localhost:8080
- Mock Agent: http://localhost:9000
- Redis: localhost:6379
- PostgreSQL: localhost:5432
- Test Frontend: http://localhost:3001

### Without Docker

```bash
# Download dependencies
go mod download

# Run
make dev

# Or directly
CONFIG_PATH=configs/config.dev.yaml go run ./cmd/server
```

## Project Structure

```
api/
├── cmd/server/           # Entry point
├── internal/
│   ├── agents/          # Registry, REST/A2A clients, permissions
│   ├── auth/            # JWT validation, Keycloak provider
│   ├── config/          # YAML loading with env vars
│   ├── handlers/        # HTTP handlers (agents, chat, sessions, read state)
│   ├── middleware/      # Auth, security headers, logging, rate limiting
│   ├── models/          # Types (Agent, Session, AG-UI events)
│   ├── proxy/           # Multiplexer (REST, A2A, ADK)
│   ├── store/           # Redis session + read state persistence
│   ├── repository/      # PostgreSQL data access
│   ├── service/         # Business logic
│   ├── pubsub/          # Real-time events (Redis Pub/Sub)
│   └── server/          # Route setup
├── configs/
│   ├── config.yaml      # Main configuration
│   └── config.dev.yaml  # Development configuration
├── docker/              # Docker Compose
└── test/mock-agent/     # Test agent
```

## Configuration

The API is configured via YAML files:

```yaml
server:
  port: 8080
  read_timeout: 30s
  write_timeout: 30s

auth:
  enabled: false  # true for production
  keycloak_issuer: "https://keycloak.example.com/realms/myrealm"
  jwks_cache_ttl: 3600

logging:
  level: debug
  format: json

agents:
  - id: mock-agent
    name: "Mock Agent"
    description: "Test agent"
    category: "testing"
    protocol: rest
    endpoint: http://mock-agent:9000/chat
    forward_authorization: true
    allowed_groups:
      - "*"  # Access for everyone
    health_check:
      enabled: true
      endpoint: /health
      interval_seconds: 30
```

### Environment Variables

Use `${ENV:VAR_NAME}` syntax in YAML:

```yaml
auth:
  keycloak_issuer: "${ENV:KEYCLOAK_ISSUER}"
```

## API

### Agents

```
GET /api/agents              # List agents (filtered by permissions)
GET /api/agents/{id}         # Agent details
GET /api/agents/{id}/health  # Health status
```

### Chat

```
POST /api/agents/{id}/chat
Content-Type: application/json

{
  "messages": [
    {"role": "user", "content": "Hello"}
  ],
  "session_id": "optional-uuid"
}
```

Response (SSE with AG-UI):

```
data: {"type":"RUN_STARTED","threadId":"...","runId":"..."}
data: {"type":"TEXT_MESSAGE_START","messageId":"...","role":"assistant"}
data: {"type":"TEXT_MESSAGE_CONTENT","messageId":"...","delta":"Hello!"}
data: {"type":"TEXT_MESSAGE_END","messageId":"..."}
data: {"type":"RUN_FINISHED","threadId":"...","runId":"..."}
```

### Sessions (Redis-backed)

```
GET    /api/agents/{id}/sessions              # List sessions
GET    /api/agents/{id}/sessions/{sessionId}  # Get session with messages
PATCH  /api/agents/{id}/sessions/{sessionId}  # Rename session
DELETE /api/agents/{id}/sessions/{sessionId}  # Delete session
```

### Read State

```
GET /api/read-state                    # Get read counts for all sessions
PUT /api/read-state/{sessionId}        # Mark session as read
PUT /api/read-state                    # Batch update (migration)
```

### Health

```
GET /health        # Liveness
GET /health/ready  # Readiness (checks agent connections)
```

## AG-UI Protocol

The API emits AG-UI events via SSE:

| Event | Description |
|-------|-------------|
| `RUN_STARTED` | Processing start |
| `TEXT_MESSAGE_START` | Assistant message start |
| `TEXT_MESSAGE_CONTENT` | Text chunk (streaming) |
| `TEXT_MESSAGE_END` | Message end |
| `TOOL_CALL_START` | Tool call start |
| `TOOL_CALL_ARGS` | Tool call arguments (streaming) |
| `TOOL_CALL_END` | Tool call end |
| `RUN_FINISHED` | Processing end |
| `RUN_ERROR` | Error during processing |

## Agent Protocols

### REST (Custom)

The agent must accept POST with body:

```json
{
  "messages": [{"role": "user", "content": "..."}]
}
```

And respond with:
- **SSE**: API translates legacy events to AG-UI
- **JSON**: API converts to AG-UI events

### A2A (Agent-to-Agent)

JSON-RPC protocol with methods:
- `tasks/send`: Create task
- `tasks/get`: Get status/result

The API polls and converts to AG-UI.

## Development Commands

```bash
# Build
go build ./...
go build -o bin/server ./cmd/server

# Run
make dev                    # With hot reload (requires air)
CONFIG_PATH=... go run ./cmd/server

# Tests
make test                   # All tests
go test ./internal/store/... -v  # Specific tests
make test-coverage          # With coverage

# Lint
make lint
make lint-fix
```

## Docker

```bash
cd docker

# Start
docker compose up -d

# Logs
docker compose logs -f api
docker compose logs -f mock-agent

# Rebuild
docker compose up -d --build

# Stop
docker compose down
```

## License

MIT
