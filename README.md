<p align="center">
  <img src="docs/agentgram-logo.svg" alt="Agentgram" width="64" height="64" />
</p>

<h1 align="center">Agentgram</h1>

<p align="center">
  A unified interface for interacting with multiple AI agents. The system provides a consistent chat experience regardless of the protocol each agent uses.
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License: MIT" /></a>
  <img src="https://img.shields.io/badge/Go-1.25-00ADD8.svg" alt="Go 1.25" />
  <img src="https://img.shields.io/badge/Next.js-16-000000.svg" alt="Next.js 16" />
</p>

## Features

- **Multi-agent**: Connect to multiple AI agents from a single interface
- **AG-UI protocol**: SSE streaming with standard AG-UI events
- **Session management**: Persistent conversation history per agent
- **Authentication**: Keycloak JWT integration (optional)
- **Permissions**: Access control by Google Workspace groups and individual users
- **Protocol abstraction**: Support for Custom, A2A, and ADK agents

## Architecture

```
┌──────────────────────┐     ┌─────────────────────┐     ┌─────────────────┐
│        Web           │────>│        API          │────>│  Agent (Custom) │
│  (Next.js+SSE/AG-UI) │<────│   (Go Multiplexer)  │<────│  SSE or JSON    │
└──────────────────────┘     │                     │     └─────────────────┘
         ↑                   │  - JWT Auth         │     ┌─────────────────┐
    AG-UI Events             │  - Permissions      │────>│  Agent (A2A)    │
                             │  - AG-UI Response   │<────│  JSON-RPC       │
                             │  - Session Proxy    │     └─────────────────┘
                             └─────────────────────┘
```

## Quick Start

### Requirements

- Node.js 22+
- Go 1.25+
- Docker & Docker Compose

### Installation

```bash
# Clone the repository
git clone https://github.com/dfradehubs/agentgram.git
cd agentgram

# Install dependencies
make install
```

### Development

```bash
# Start the full development environment
make dev
```

This will start:
- **API** at http://localhost:8080 (via Docker)
- **Mock Agent** at http://localhost:9000 (for testing)
- **Web** at http://localhost:3000

### Available Commands

```bash
make help           # View all available commands

# Development
make dev            # Start full environment (Docker API + Next.js)
make dev-docker     # Start everything with Docker

# Individual services
make api            # API only (requires local Go)
make web            # Web only

# Docker
make docker-up      # Start Docker services
make docker-down    # Stop Docker services
make docker-logs    # View logs

# Testing
make test           # Run all tests
make lint           # Run linters

# Cleanup
make clean          # Clean build artifacts
```

## Project Structure

```
agentgram/
├── api/                        # Go API (multiplexer)
│   ├── cmd/server/            # Entry point
│   ├── internal/
│   │   ├── proxy/             # Proxy to agents (REST, A2A, ADK) + SSE writer
│   │   ├── handlers/          # HTTP handlers (chat, sessions, admin, MCP, custom agents)
│   │   ├── models/            # Data types (AG-UI, sessions, agents)
│   │   ├── agents/            # Registry, clients, permissions, health checker
│   │   ├── auth/              # JWT, OIDC, GitHub OAuth
│   │   ├── middleware/        # Auth, security headers, rate limit, body limit
│   │   ├── mcp/               # MCP server registry and client
│   │   ├── customagent/       # Custom agent runtime and tools
│   │   ├── store/             # Redis session store
│   │   ├── repository/        # PostgreSQL repositories
│   │   └── config/            # YAML loader with ${ENV:VAR}
│   ├── configs/               # YAML configuration
│   ├── migrations/            # PostgreSQL migrations
│   ├── docker/                # Docker Compose for development
│   └── test/mock-agent/       # Mock agent for testing
├── web/                        # Next.js web
│   └── src/
│       ├── app/               # App Router (pages, admin, auth proxy)
│       ├── components/        # React components (chat, sidebar, admin, MCP)
│       ├── contexts/          # Contexts (Agent, Session, Background Stream, MCP, User)
│       ├── hooks/             # Custom hooks (useChat, useMCPChat)
│       └── lib/               # API client, types, i18n, logging, PDF export
├── docs/                      # Documentation
├── Makefile                   # Development commands
└── README.md                  # This documentation
```

## API

See [docs/API.md](docs/API.md) for the complete API documentation.

### Chat

```http
POST /api/agents/{agentId}/chat
Content-Type: application/json

{
  "messages": [
    {"role": "user", "content": "Hello"}
  ],
  "session_id": "optional-uuid"
}
```

Response: SSE with AG-UI events

```
data: {"type":"RUN_STARTED","threadId":"...","runId":"..."}
data: {"type":"TEXT_MESSAGE_START","messageId":"...","role":"assistant"}
data: {"type":"TEXT_MESSAGE_CONTENT","messageId":"...","delta":"Hello!"}
data: {"type":"TEXT_MESSAGE_END","messageId":"..."}
data: {"type":"RUN_FINISHED","threadId":"...","runId":"..."}
```

### Agents

```http
GET /api/agents              # List available agents
GET /api/agents/{id}         # Get details of an agent
GET /api/agents/{id}/health  # Agent health status
```

### Sessions

```http
GET    /api/agents/{id}/sessions              # List sessions
GET    /api/agents/{id}/sessions/{sessionId}  # Get session with messages
PATCH  /api/agents/{id}/sessions/{sessionId}  # Rename session
DELETE /api/agents/{id}/sessions/{sessionId}  # Delete session
```

## Configuration

### API

The API is configured through YAML files. See `api/configs/config.yaml`:

```yaml
server:
  port: 8080

auth:
  enabled: false  # Enable for production

agents:
  - id: my-agent
    name: "My Agent"
    protocol: custom
    endpoint: http://my-agent:9000/chat
    allowed_groups:
      - "*"  # Public access
```

### Web

Environment variables in `web/.env.local`:

```env
NEXT_PUBLIC_API_URL=http://localhost:8080
```

## Tech Stack

### API
- **Go 1.25** - Main language
- **Chi** - HTTP router
- **Zap** - Structured logging

### Web
- **Next.js 16** - React framework
- **SSE/AG-UI** - Direct streaming via fetch + ReadableStream
- **Tailwind CSS 4** - Styling
- **TypeScript** - Static typing
- **Pino** - Server-side logging (JSON in production)

## Development

### Adding a new agent

1. Add the configuration in `api/configs/config.yaml`
2. The agent must implement one of the supported protocols:
   - **Custom**: An endpoint that accepts a POST with messages and responds with SSE or JSON
   - **A2A**: Agent-to-Agent protocol with JSON-RPC
   - **ADK**: Google ADK framework with REST SSE

### Implementing sessions in an agent

Agents that support sessions must implement:

```
GET    /api/sessions                    # List the user's sessions
GET    /api/sessions/{sessionId}        # Get session with messages
PATCH  /api/sessions/{sessionId}        # Rename session
DELETE /api/sessions/{sessionId}        # Delete session
```

See `api/test/mock-agent/main.go` as a reference.

## Claude Code (MCP)

Agentgram exposes an MCP endpoint that lets you use the deployed agents as tools directly from Claude Code.

### Configure

Agentgram implements the full OAuth flow for MCP: protected resource metadata (RFC 9728), authorization server discovery (RFC 8414), and **Dynamic Client Registration (RFC 7591)**. The client discovers and registers everything automatically, so **there is no need to configure `clientId`, scopes, or endpoints by hand**.

**Claude Code:**

```bash
claude mcp add --transport http agentgram https://agentgram.example.com/mcp
```

**Cursor** — in `~/.cursor/mcp.json` (global) or `.cursor/mcp.json` (project):

```json
{
  "mcpServers": {
    "agentgram": {
      "type": "http",
      "url": "https://agentgram.example.com/mcp"
    }
  }
}
```

The same configuration works for **Cursor automations** (background agents): they share the `mcp.json`, so once agentgram is added it is also available to the automations.

### Authenticate

Inside Claude Code:

```
/mcp
```

Select `agentgram` and authenticate with your Google account (Keycloak). You only need to do this once.

### Usage

Once connected, Claude Code can use the agents directly:

- **Ask an agent**: "Ask kube-agent how many pods there are in the istio-system namespace"
- **Search logs**: "Ask logs-agent for the errors in the last 30 minutes"
- **Analyze traces**: "Ask traces-agent for traces with 5xx errors"
- **Query metrics**: "Ask metrics-agent for the cluster's CPU usage"

The available agents are discovered automatically based on your permissions.

### Recommended configuration

Add this to `~/.claude/settings.json` for a better experience:

```json
{
  "env": {
    "MAX_MCP_OUTPUT_TOKENS": "50000"
  },
  "permissions": {
    "allow": [
      "mcp__agentgram__*"
    ]
  }
}
```

- **`MAX_MCP_OUTPUT_TOKENS`**: Increases the MCP response token limit (default 25K). Agents like logs-agent or sentry-agent return long responses with tables and breakdowns.
- **`permissions.allow`**: Allows the agentgram tools without a confirmation prompt. Without this, every call to an agent requires manual approval.

### Known limitations

- **Sequential execution**: Claude Code runs MCP calls one after another, not in parallel. If you ask it to query two agents at once, the second one will wait until the first finishes. This is a limitation of the MCP client's HTTP streamable transport.
- **Timeout on long responses**: The MCP SDK has a 60s default timeout. Agentgram sends progress notifications every 15s to keep the connection alive, but if the agent takes longer than expected, retry with a more specific query.

See [docs/MCP.md](docs/MCP.md) for full documentation, Cursor configuration, and more examples.

## Additional Documentation

- [MCP Integration](docs/MCP.md) - Integrate agents into Claude Code, Cursor, and other MCP clients
- [API Documentation](docs/API.md) - Complete API documentation
- [API CLAUDE.md](api/CLAUDE.md) - Guide for API development
- [Web CLAUDE.md](web/CLAUDE.md) - Guide for web development

## License

Distributed under the MIT license. See the [LICENSE](LICENSE) file for more information.
