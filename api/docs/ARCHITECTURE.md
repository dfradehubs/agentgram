# Architecture - Agentgram API

## Overview

The API acts as an **agent multiplexer**, allowing users to interact with multiple remote agents through a unified interface.

```
┌─────────────┐     ┌─────────────────┐     ┌─────────────┐
│     Web     │────>│      API        │────>│   Agent 1   │
│             │<────│  (Multiplexer)  │<────│   (REST)    │
└─────────────┘     │                 │     └─────────────┘
                    │                 │     ┌─────────────┐
                    │                 │────>│   Agent 2   │
                    │                 │<────│   (A2A)     │
                    └─────────────────┘     └─────────────┘
```

## Main Components

### 1. HTTP Server (`internal/server/`)

- **server.go**: HTTP server with graceful shutdown
- **routes.go**: Route definitions using Chi router

### 2. Authentication (`internal/auth/`)

- **jwt.go**: JWT token validation
- **keycloak.go**: Keycloak JWKS cache
- **claims.go**: Claims extraction (groups, email)

### 3. Agents (`internal/agents/`)

- **registry.go**: Central agent registry
- **permissions.go**: Authorization logic
- **health.go**: Periodic health checks
- **rest_client.go**: HTTP client for REST agents
- **a2a_client.go**: Client for A2A agents

### 4. Proxy (`internal/proxy/`)

- **proxy.go**: Main multiplexer
- **rest.go**: Proxy for REST agents (direct SSE or JSON→SSE)
- **a2a.go**: Proxy for A2A agents (polling→SSE)
- **sse.go**: Helpers for writing SSE events

### 5. Middleware (`internal/middleware/`)

- **auth.go**: JWT validation
- **security.go**: Security headers
- **logging.go**: Request logging
- **ratelimit.go**: Per-agent/user rate limiting

## Request Flow

```
1. Request arrives at server
   ↓
2. Logging middleware records the request
   ↓
3. Security headers middleware applies headers
   ↓
4. Body limit middleware enforces size limit
   ↓
5. Auth middleware validates JWT
   ↓
6. Handler extracts agentId from URL
   ↓
7. Registry returns agent configuration
   ↓
8. Permissions verifies user access
   ↓
9. Proxy routes by protocol (REST/A2A)
   ↓
10. SSE response to web
```

## Supported Protocols

### REST

Agents that expose a REST API for chat. They can respond with:
- **Direct SSE**: API proxies directly
- **Sync JSON**: API converts to SSE

```go
type RESTAgent struct {
    Endpoint string            // Endpoint URL
    Headers  map[string]string // Additional headers
}
```

### A2A (Agent-to-Agent)

Standard protocol for inter-agent communication. Uses JSON-RPC 2.0:

1. **tasks/send**: Sends a new task
2. **tasks/get**: Gets task status

The API polls and converts responses to SSE.

```go
type A2AAgent struct {
    Endpoint      string // A2A server URL
    AgentCardPath string // Path to agent-card.json
    Polling       struct {
        IntervalMS     int
        TimeoutSeconds int
    }
}
```

## Permission Model

Permissions are defined per agent using two lists:

1. **allowed_groups**: Google Workspace groups
2. **allowed_users**: Specific emails

```yaml
allowed_groups:
  - "google-workspace/sre@example.com"
allowed_users:
  - "admin@example.com"
```

Access is granted if the user belongs to **at least one** of the groups or is in the users list.

## Configuration

### Environment Variables

The `${ENV:VARIABLE}` syntax allows referencing environment variables in YAML files:

```yaml
endpoint: "${ENV:AGENT_ENDPOINT}"
api_key: "${ENV:SECRET_KEY}"
```

### Configuration Files

- `configs/config.yaml`: General server configuration

## Security

1. **JWT Validation**: Tokens validated against Keycloak JWKS
2. **JWKS Cache**: Cache with configurable TTL to reduce latency
3. **Rate Limiting**: Per agent and user
4. **Security Headers**: Applied globally
5. **Header Forwarding**: Optional, allows forwarding JWT to agent

## Observability

### Logging

Structured logging with Zap:
- Configurable level (debug, info, warn, error)
- JSON or console format
- Request logging with duration and status

### Health Checks

- `/health`: Liveness probe
- `/health/ready`: Readiness probe (checks agents)
- Periodic health checks to each agent

## Scalability

The API is stateless and can scale horizontally. Considerations:

1. **Rate Limiting**: In-memory, consider Redis for multiple instances
2. **JWKS Cache**: In-memory, each instance maintains its own cache
3. **Health Checks**: Each instance runs its own checks

## Development

### Project Structure

```
api/
├── cmd/server/       # Entry point
├── internal/         # Private code
│   ├── config/       # Configuration
│   ├── server/       # HTTP server
│   ├── handlers/     # HTTP handlers
│   ├── middleware/    # Middlewares
│   ├── auth/         # Authentication
│   ├── agents/       # Agent management
│   ├── proxy/        # Proxy/multiplexer
│   ├── store/        # Redis session store
│   ├── repository/   # PostgreSQL data access
│   ├── service/      # Business logic
│   ├── pubsub/       # Real-time events (Redis Pub/Sub)
│   └── models/       # Data types
├── configs/          # Configuration files
├── docker/           # Docker files
├── test/             # Tests
└── docs/             # Documentation
```

### Testing

- **Unit tests**: `go test ./...`
- **Integration tests**: `go test ./test/integration/... -tags=integration`
- **Mock agent**: `test/mock-agent/` for local testing
