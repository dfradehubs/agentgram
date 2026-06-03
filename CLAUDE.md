# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Agentgram** is a unified interface for interacting with multiple AI agents. The system consists of:

- **API** (Go): Multiplexer that connects to remote agents, handles authentication (Keycloak JWT), permissions (Google Workspace groups/users), and emits **AG-UI protocol** events
- **Web** (Next.js + SSE/AG-UI): User interface for chatting with agents

## Repository Structure

```
agentgram/
├── api/              # Go API (multiplexer)
│   ├── cmd/server/   # Entry point
│   ├── internal/     # Core packages
│   ├── configs/      # YAML configuration
│   ├── docker/       # Docker compose for dev environment
│   └── test/         # Mock agent and test frontend
├── web/              # Next.js web with SSE/AG-UI
└── Makefile          # Root commands
```

## Quick Start

```bash
# Full environment with Docker (api + mock agents + test frontend)
make docker-up

# Or run separately:
make api              # Terminal 1: Go API on :8080
make web              # Terminal 2: Next.js on :3000
```

## Architecture

```
┌──────────────────────┐     ┌─────────────────────┐     ┌─────────────────┐
│        Web           │────>│        API          │────>│  Agent (Custom) │
│  (Next.js+SSE/AG-UI)│<────│   (Go Multiplexer)  │<────│  SSE or JSON    │
└──────────────────────┘     │                     │     └─────────────────┘
         ↑                   │  - JWT Auth         │     ┌─────────────────┐
    AG-UI Events             │  - Permissions      │────>│  Agent (A2A)    │
                             │  - AG-UI Response   │<────│  JSON-RPC       │
                             │  - Session Store    │     └─────────────────┘
                             └─────────────────────┘
```

### Key Design Decisions

1. **AG-UI Protocol**: API emits AG-UI events (`RUN_STARTED`, `TEXT_MESSAGE_*`, `RUN_FINISHED`) that SSE/AG-UI consumes directly
2. **Protocol Abstraction**: Web doesn't know if agent uses REST or A2A - API handles conversion
3. **Sessions in Redis**: The API manages sessions directly via `store.SessionStore` (Redis); agents are stateless with respect to sessions
4. **Permission Model**: Access controlled by Google Workspace groups + individual users per agent
5. **Config as YAML**: All configuration in YAML files with `${ENV:VAR}` syntax for secrets
6. **Security Layers**: Security headers, body size limits, JWT audience+nonce validation, rate limiting, sanitized error responses

## API Contract

### Chat Endpoint

```
POST /api/agents/{agentId}/chat
Content-Type: application/json
Authorization: Bearer <jwt>

{
  "messages": [
    {"role": "user", "content": "Hello"},
    {"role": "assistant", "content": "Hi!"},
    {"role": "user", "content": "How are you?"}
  ],
  "session_id": "optional-session-uuid"
}
```

### AG-UI SSE Response

```
data: {"type":"RUN_STARTED","threadId":"...","runId":"..."}
data: {"type":"TEXT_MESSAGE_START","messageId":"...","role":"assistant"}
data: {"type":"TEXT_MESSAGE_CONTENT","messageId":"...","delta":"Hello"}
data: {"type":"TEXT_MESSAGE_CONTENT","messageId":"...","delta":", how can I help?"}
data: {"type":"TEXT_MESSAGE_END","messageId":"..."}
data: {"type":"RUN_FINISHED","threadId":"...","runId":"..."}
```

Event types: `RUN_STARTED`, `RUN_FINISHED`, `RUN_ERROR`, `TEXT_MESSAGE_START`, `TEXT_MESSAGE_CONTENT`, `TEXT_MESSAGE_END`, `TOOL_CALL_START`, `TOOL_CALL_ARGS`, `TOOL_CALL_END`, `CUSTOM`

### Sessions API (Stored in Redis)

```
GET    /api/agents/{agentId}/sessions              # List sessions
GET    /api/agents/{agentId}/sessions/{sessionId}   # Get session with messages
PATCH  /api/agents/{agentId}/sessions/{sessionId}   # Rename session
DELETE /api/agents/{agentId}/sessions/{sessionId}   # Delete session
```

## Development

See individual CLAUDE.md files:
- `api/CLAUDE.md` - Go API details
- `web/CLAUDE.md` - Next.js web details

## Test Environment

`make docker-up` starts:
- API on :8080 (auth disabled for testing)
- Mock agent on :9000 (REST SSE + A2A JSON-RPC + Sessions API)
- Redis on :6379 (session storage + pub/sub)
- PostgreSQL on :5432 (groups + persistent data)
- Test frontend on :3001 (simple HTML/JS for verification)
