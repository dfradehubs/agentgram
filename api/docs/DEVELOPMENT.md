# Development Guide - Agentgram API

## Requirements

- Go 1.25 or higher
- Docker and Docker Compose (for full local environment)
- Make

## Initial Setup

```bash
# Clone and enter the directory
cd api

# Download dependencies
go mod download
```

## Running in Development

### API only

```bash
# Without hot reload
make dev

# With hot reload (requires air)
make dev-watch
```

### Full environment with Docker

```bash
# Start all services
make docker-up

# View logs
make docker-logs

# Stop
make docker-down
```

Available services:
- API: http://localhost:8080
- Mock Agent: http://localhost:9000
- Redis: localhost:6379
- PostgreSQL: localhost:5432
- Test Frontend: http://localhost:3001

## Tests

```bash
# Unit tests
make test

# With coverage
make test-coverage

# Integration tests (requires docker-up)
make test-integration
```

## Linting

```bash
# Check
make lint

# Auto-fix
make lint-fix
```

## Code Structure

### Adding a new endpoint

1. Create handler in `internal/handlers/`
2. Register route in `internal/server/routes.go`
3. Add tests

### Adding a new agent type

1. Implement client in `internal/agents/`
2. Add case in `internal/proxy/proxy.go`
3. Update documentation

### Adding a new middleware

1. Create in `internal/middleware/`
2. Register in `internal/server/routes.go`

## Agent Configuration

Agents are configured in `configs/config.yaml`:

```yaml
agents:
  - id: "my-agent"
    name: "My Agent"
    description: "Description"
    category: "category"
    protocol: "custom"  # or "a2a"
    endpoint: "${ENV:MY_AGENT_ENDPOINT}"

    headers:
      X-Api-Key: "${ENV:MY_AGENT_KEY}"

    forward_authorization: true

    allowed_groups:
      - "google-workspace/my-group@company.com"
    allowed_users:
      - "user@company.com"

    rate_limit:
      requests_per_minute: 60

    health_check:
      enabled: true
      endpoint: "/health"
      interval_seconds: 30
      timeout_seconds: 5

    metadata:
      icon: "my-icon"
      color: "#FF0000"
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | Server port | 8080 |
| `CONFIG_PATH` | Path to config file | ./configs/config.yaml |
| `POSTGRES_SSLMODE` | Database SSL mode | require |
| `LOG_LEVEL` | Log level | info |

## Debugging

### Logs

In development, use `LOG_LEVEL=debug` for verbose logs:

```bash
LOG_LEVEL=debug make dev
```

### Testing endpoints manually

```bash
# Health
curl http://localhost:8080/health

# List agents (requires JWT)
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/agents

# Chat (SSE)
curl -N -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"messages":[{"role":"user","content":"hello"}]}' \
  http://localhost:8080/api/agents/agent-sales/chat
```

## Conventions

### Code

- Use `gofmt` for formatting
- Follow [Effective Go](https://golang.org/doc/effective_go)
- Document exported functions
- Use error wrapping with `fmt.Errorf("context: %w", err)`

### Commits

- Use conventional commits: `feat:`, `fix:`, `docs:`, etc.
- Include tests in the same commit as the code

### Pull Requests

- Clear description of the change
- Tests passing
- Lint without errors
- Documentation updated if needed

## Troubleshooting

### "JWKS fetch failed"

- Verify Keycloak is running
- Verify `KEYCLOAK_ISSUER` points to the correct URL
- Verify the realm exists

### "Agent not found"

- Verify the agent is in config
- Verify the agent endpoint is configured

### "Access denied"

- Verify the user has the correct groups/email in the JWT
- Verify the `allowed_groups`/`allowed_users` configuration for the agent

### SSE not working

- Verify there is no proxy/nginx buffering
- Verify response headers: `Content-Type: text/event-stream`
- Verify the client supports SSE
