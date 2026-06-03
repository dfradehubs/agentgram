# Agentgram API Reference

## Overview

Agentgram API is a Go multiplexer that connects to multiple remote AI agents and MCP servers. It handles authentication, permissions, session management, and protocol translation, exposing a unified SSE interface using the AG-UI protocol.

- **Base URL**: `http(s)://<host>:8080`
- **Content-Type**: `application/json` for request/response bodies; `text/event-stream` for SSE streaming endpoints
- **Authentication**: OIDC (Keycloak) with session cookies, JWT bearer tokens, and optional GitHub OAuth for agent token forwarding

---

## Authentication

### OIDC Flow

1. Frontend redirects to `GET /auth/login`
2. User authenticates with Keycloak
3. Keycloak redirects to `GET /auth/callback` with authorization code
4. API exchanges code for tokens, creates a server-side session in Redis, and sets an `auth_session` HttpOnly cookie
5. Subsequent API requests are authenticated via the session cookie or JWT `Authorization: Bearer <token>` header

### Session Cookie

- **Name**: `auth_session`
- **Attributes**: `HttpOnly`, `SameSite=Lax`, `Path=/`
- **Max-Age**: Configurable (default 86400 seconds)
- **Secure**: Configurable via `auth.cookie_secure`

### JWT Format

The JWT is issued by Keycloak and validated using JWKS. Claims include:
- `email` - user email
- `groups` - list of Google Workspace groups
- `sub` - subject identifier

### GitHub OAuth (Optional)

Users can link their GitHub account to forward tokens to agents that require GitHub access (e.g., coding agents). The GitHub token is stored in the auth session alongside the OIDC tokens.

---

## Error Format

All error responses use the following JSON structure:

```json
{
  "error": "description of what went wrong"
}
```

### Common HTTP Status Codes

| Status | Meaning |
|--------|---------|
| 200 | Success |
| 201 | Created |
| 204 | No Content (successful deletion) |
| 400 | Bad Request (invalid input) |
| 401 | Unauthorized (missing or invalid auth) |
| 403 | Forbidden (insufficient permissions) |
| 404 | Not Found |
| 500 | Internal Server Error |
| 503 | Service Unavailable |

---

## AG-UI SSE Protocol

All chat endpoints (`/api/agents/{id}/chat`, `/api/sessions/{id}/broadcast`, `/api/sessions/{id}/conversation`, `/api/mcp/servers/{id}/chat`, `/api/mcp/chat`) respond with `text/event-stream` using the AG-UI protocol.

### Event Types

| Event Type | Description |
|------------|-------------|
| `RUN_STARTED` | Signals the beginning of a run |
| `RUN_FINISHED` | Signals the end of a run |
| `RUN_ERROR` | Signals an error during the run |
| `TEXT_MESSAGE_START` | Start of a text message from an assistant |
| `TEXT_MESSAGE_CONTENT` | A chunk of streaming text content |
| `TEXT_MESSAGE_END` | End of a text message |
| `TOOL_CALL_START` | Start of a tool call |
| `TOOL_CALL_ARGS` | Arguments for a tool call (streamed) |
| `TOOL_CALL_END` | End of a tool call with result |
| `CUSTOM` | Custom event (e.g., `CONVERSATION_STEP`) |

### Event Structures

```
data: {"type":"RUN_STARTED","threadId":"<session-id>","runId":"<uuid>"}

data: {"type":"TEXT_MESSAGE_START","messageId":"<uuid>","role":"assistant","agentId":"<optional>","isThinking":false}

data: {"type":"TEXT_MESSAGE_CONTENT","messageId":"<uuid>","delta":"Hello","agentId":"<optional>"}

data: {"type":"TEXT_MESSAGE_END","messageId":"<uuid>","agentId":"<optional>"}

data: {"type":"TOOL_CALL_START","toolCallId":"<id>","toolName":"search","serverId":"<optional>"}

data: {"type":"TOOL_CALL_ARGS","toolCallId":"<id>","delta":"{\"query\":\"test\"}"}

data: {"type":"TOOL_CALL_END","toolCallId":"<id>","result":"..."}

data: {"type":"RUN_ERROR","message":"error description","code":"<optional>"}

data: {"type":"RUN_FINISHED","threadId":"<session-id>","runId":"<uuid>"}
```

### Complete Example

```
data: {"type":"RUN_STARTED","threadId":"sess-123","runId":"run-456"}

data: {"type":"TEXT_MESSAGE_START","messageId":"msg-789","role":"assistant"}

data: {"type":"TEXT_MESSAGE_CONTENT","messageId":"msg-789","delta":"Hello"}

data: {"type":"TEXT_MESSAGE_CONTENT","messageId":"msg-789","delta":", how can I help?"}

data: {"type":"TEXT_MESSAGE_END","messageId":"msg-789"}

data: {"type":"RUN_FINISHED","threadId":"sess-123","runId":"run-456"}
```

### Multi-Agent Events

In broadcast and conversation modes, events include an `agentId` field to identify which agent is responding. The `isThinking` field in `TEXT_MESSAGE_START` marks intermediate thinking steps.

### Custom Events

```
data: {"type":"CUSTOM","subType":"CONVERSATION_STEP","data":{"agentId":"agent-1","stepIndex":0,"totalSteps":3,"isUserTurn":false}}
```

---

## Permission Model

Access to agents and MCP servers is controlled by two lists:

- **`allowed_groups`**: Google Workspace groups (matched against JWT `groups` claim)
- **`allowed_users`**: Individual email addresses

### Rules

- If both lists are empty, the resource is accessible to all authenticated users
- The wildcard `*` in either list grants access to all authenticated users
- A user needs to match at least one group OR one user entry to gain access
- Admin users (role `admin` in the database) can access admin API endpoints regardless of group membership

---

## Public Endpoints

No authentication required.

### GET /health

Liveness probe.

**Response** `200`:
```json
{
  "status": "ok"
}
```

### GET /health/ready

Readiness probe. Returns `ready` if at least one agent is healthy or all agents have unknown status (no health check configured).

**Response** `200`:
```json
{
  "status": "ready",
  "details": {
    "total_agents": 3,
    "healthy_agents": 2
  }
}
```

**Response** `503`:
```json
{
  "status": "degraded",
  "details": {
    "total_agents": 3,
    "healthy_agents": 0,
    "unhealthy_agents": ["agent-1", "agent-2", "agent-3"]
  }
}
```

### GET /api/config

Returns public configuration (feature flags and available LLM models). No secrets are exposed.

**Response** `200`:
```json
{
  "features": {
    "summarizer_enabled": true
  },
  "available_models": [
    {
      "id": "gpt-4o",
      "name": "GPT-4o",
      "provider": "openai",
      "default": true
    }
  ]
}
```

---

## Auth OIDC Endpoints

### GET /auth/login

Redirects the user to Keycloak for OIDC authentication.

**Response**: `302 Found` redirect to Keycloak authorization URL.

### GET /auth/callback

Handles the OIDC callback from Keycloak. Exchanges the authorization code for tokens, creates a session, sets the `auth_session` cookie, and redirects to `/`.

**Query Parameters**:
- `code` (required) - Authorization code
- `state` (required) - CSRF state token

**Response**: `302 Found` redirect to `/`.

**Errors**: `400` (missing code/state, invalid state), `401` (authentication failed), `500` (token exchange failed).

### POST /auth/logout

Clears the auth session and returns the Keycloak logout URL for SSO logout.

**Response** `200`:
```json
{
  "ok": true,
  "logout_url": "https://keycloak.example.com/realms/my-realm/protocol/openid-connect/logout?..."
}
```

### GET /auth/session

Returns the current user's session info based on the `auth_session` cookie.

**Response** `200` (authenticated):
```json
{
  "authenticated": true,
  "email": "user@example.com",
  "groups": ["group-a", "group-b"],
  "github_connected": true,
  "github_username": "octocat"
}
```

**Response** `200` (not authenticated):
```json
{
  "authenticated": false
}
```

---

## Auth GitHub OAuth Endpoints

### GET /auth/github/login

Redirects the user to GitHub for OAuth authorization. Requires an active OIDC session.

**Response**: `302 Found` redirect to GitHub authorization URL.

**Errors**: `404` (GitHub OAuth not configured), `500` (internal error).

### GET /auth/github/callback

Handles the GitHub OAuth callback. Stores the GitHub token in the user's existing auth session.

**Query Parameters**:
- `code` (required) - Authorization code
- `state` (required) - CSRF state token

**Response**: `302 Found` redirect to `/`.

### GET /auth/github/status

Returns the current GitHub connection status.

**Response** `200`:
```json
{
  "connected": true,
  "username": "octocat"
}
```

### POST /auth/github/disconnect

Removes the GitHub token from the user's session.

**Response** `200`:
```json
{
  "ok": true
}
```

---

## User Endpoints

Authentication required.

### GET /api/me

Returns the current user's profile information.

**Response** `200`:
```json
{
  "email": "user@example.com",
  "groups": ["group-a", "group-b"],
  "is_admin": false
}
```

---

## Agent Endpoints

Authentication required. Agents are filtered by user permissions.

### GET /api/agents

Lists all agents the authenticated user has access to.

**Response** `200`:
```json
{
  "agents": [
    {
      "id": "my-agent",
      "name": "My Agent",
      "description": "A helpful agent",
      "category": "general",
      "protocol": "rest",
      "status": "healthy",
      "require_github_token": false
    }
  ]
}
```

### GET /api/agents/{agentId}

Returns a single agent by ID.

**Response** `200`:
```json
{
  "id": "my-agent",
  "name": "My Agent",
  "description": "A helpful agent",
  "category": "general",
  "protocol": "rest",
  "status": "healthy",
  "require_github_token": false
}
```

**Errors**: `400` (missing ID), `403` (access denied), `404` (not found).

### GET /api/agents/{agentId}/health

Returns the health status of a specific agent.

**Response** `200`:
```json
{
  "status": "healthy",
  "details": {
    "agent_id": "my-agent",
    "name": "My Agent",
    "protocol": "rest"
  }
}
```

**Errors**: `400` (missing ID), `403` (access denied), `404` (not found).

---

## Chat SSE Endpoint

Authentication required. Agent-level permission check.

### POST /api/agents/{agentId}/chat

Sends a message to an agent and streams the response as AG-UI SSE events. If no `session_id` is provided, a new session is created automatically.

**Request Body**:
```json
{
  "messages": [
    {
      "role": "user",
      "content": "Hello, how are you?",
      "attachments": [
        {
          "filename": "image.png",
          "content_type": "image/png",
          "data": "<base64>"
        }
      ]
    }
  ],
  "session_id": "optional-session-uuid",
  "send_context": true,
  "summarize_context": false
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `messages` | array | yes | Conversation messages. At least one message required. |
| `messages[].role` | string | yes | `"user"`, `"assistant"`, or `"system"` |
| `messages[].content` | string | yes | Message text |
| `messages[].attachments` | array | no | File attachments (base64-encoded) |
| `session_id` | string | no | Existing session ID. If omitted, a new session is created. |
| `send_context` | boolean | no | Whether to send previous conversation context to the agent (default: true) |
| `summarize_context` | boolean | no | Whether to summarize context before sending (default: false) |

**Response**: `text/event-stream` with AG-UI events. The `threadId` in `RUN_STARTED` contains the session ID.

**Errors**: `400` (missing agent ID, invalid body, no messages), `401` (unauthorized), `403` (access denied, GitHub token required), `404` (agent not found), `500` (session creation failed).

**Note**: Agents with `require_github_token: true` will return `403` with `{"error":"github_token_required","message":"Connect your GitHub account to use this agent"}` if the user has not linked a GitHub account.

---

## Session Endpoints

Authentication required. Owner-only access (sessions are scoped to the user who created them).

### GET /api/agents/{agentId}/sessions

Lists all sessions for the authenticated user with a specific agent.

**Response** `200`:
```json
{
  "sessions": [
    {
      "session_id": "uuid",
      "session_name": "My chat",
      "user_id": "user@example.com",
      "app_name": "my-agent",
      "is_multi_agent": false,
      "created_at": 1700000000,
      "last_activity": 1700001000,
      "message_count": 5
    }
  ]
}
```

### GET /api/agents/{agentId}/sessions/{sessionId}

Returns a session with all its messages.

**Response** `200`:
```json
{
  "session_id": "uuid",
  "session_name": "My chat",
  "user_id": "user@example.com",
  "app_name": "my-agent",
  "is_multi_agent": false,
  "created_at": 1700000000,
  "last_activity": 1700001000,
  "message_count": 5,
  "messages": [
    {
      "role": "user",
      "content": "Hello"
    },
    {
      "role": "assistant",
      "content": "Hi there!",
      "tool_calls": [],
      "tool_results": [],
      "content_parts": [
        {"type": "text", "text": "Hi there!"}
      ]
    }
  ]
}
```

**Errors**: `400` (missing IDs), `403` (access denied / not owner), `404` (not found), `500`.

### PATCH /api/agents/{agentId}/sessions/{sessionId}

Renames a session.

**Request Body**:
```json
{
  "session_name": "New name"
}
```

**Response** `200`: Updated session object (same structure as GET).

**Errors**: `400` (missing fields), `403` (not owner), `404` (not found).

### DELETE /api/agents/{agentId}/sessions/{sessionId}

Deletes a session and all its messages.

**Response**: `204 No Content`

**Errors**: `400` (missing IDs), `403` (not owner), `404` (not found).

---

## Multi-Agent Session Endpoints

Authentication required. Owner-only access.

### POST /api/sessions/multi

Creates a new multi-agent session.

**Request Body**:
```json
{
  "session_name": "Optional name",
  "agent_ids": ["agent-1", "agent-2"],
  "multi_agent_mode": "broadcast",
  "sequence": ["agent-1", "user", "agent-2", "user"]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `session_name` | string | no | Session name (auto-generated from agent names if omitted) |
| `agent_ids` | array | yes | At least 2 agent IDs |
| `multi_agent_mode` | string | no | `"broadcast"` (default) or `"conversation"` |
| `sequence` | array | conditional | Required for `conversation` mode. Ordered list of agent IDs and `"user"` markers. |

**Response** `201`: Session object.

**Errors**: `400` (invalid input, fewer than 2 agents, missing sequence for conversation), `403` (access denied to an agent), `404` (agent not found).

### GET /api/sessions/multi

Lists all multi-agent sessions for the authenticated user.

**Response** `200`:
```json
{
  "sessions": [...]
}
```

### GET /api/sessions/multi/{sessionId}

Returns a multi-agent session with all messages.

**Response** `200`: Session object with messages.

**Errors**: `403` (not owner), `404` (not found).

### PATCH /api/sessions/multi/{sessionId}

Updates a multi-agent session (rename and/or update sequence).

**Request Body**:
```json
{
  "session_name": "New name",
  "sequence": ["agent-1", "user", "agent-2", "user"]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `session_name` | string | no | New session name |
| `sequence` | array | no | New sequence (only for conversation mode sessions) |

**Response** `200`: Updated session object.

**Errors**: `400` (invalid sequence), `403` (not owner), `404` (not found).

### DELETE /api/sessions/multi/{sessionId}

Deletes a multi-agent session.

**Response**: `204 No Content`

**Errors**: `403` (not owner), `404` (not found).

---

## Broadcast SSE Endpoint

Authentication required. Owner-only access to the session. Permission check on all target agents.

### POST /api/sessions/{sessionId}/broadcast

Sends a message to multiple agents in parallel. Responses are streamed and interleaved as AG-UI events with `agentId` fields.

**Request Body**:
```json
{
  "message": "What do you think about this topic?",
  "agent_ids": ["agent-1", "agent-2"],
  "send_context": true,
  "summarize_context": false
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `message` | string | yes | The message to broadcast |
| `agent_ids` | array | yes | At least 2 agent IDs to broadcast to |
| `send_context` | boolean | no | Send previous conversation context (default: true) |
| `summarize_context` | boolean | no | Summarize context before sending (default: false) |

**Response**: `text/event-stream` with AG-UI events. Each event includes an `agentId` field.

**Errors**: `400` (missing message, fewer than 2 agents, session not multi-agent), `403` (access denied), `404` (session or agent not found).

---

## Conversation SSE Endpoint

Authentication required. Owner-only access. Session must be in `conversation` mode.

### POST /api/sessions/{sessionId}/conversation

Sends a message and executes the conversation sequence. Agents are called sequentially according to the session's `sequence` definition. Execution pauses when a `"user"` step is reached.

**Request Body**:
```json
{
  "message": "Start the process",
  "send_context": true,
  "summarize_context": false
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `message` | string | yes | The user's message |
| `send_context` | boolean | no | Send previous conversation context (default: true) |
| `summarize_context` | boolean | no | Summarize context before sending (default: false) |

**Response**: `text/event-stream` with AG-UI events. Includes `CUSTOM` events with `subType: "CONVERSATION_STEP"` to indicate progress through the sequence.

**Errors**: `400` (missing message, session not in conversation mode, no sequence defined), `403` (access denied), `404` (session or agent not found).

---

## MCP Server Endpoints

Authentication required. Server-level permission check.

### GET /api/mcp/servers

Lists all MCP servers the authenticated user has access to.

**Response** `200`:
```json
{
  "servers": [
    {
      "id": "my-server",
      "name": "My MCP Server",
      "description": "A tool server",
      "transport": "sse",
      "status": "connected",
      "status_error": "",
      "tool_count": 5
    }
  ]
}
```

### GET /api/mcp/servers/{id}/tools

Lists all tools available on an MCP server.

**Response** `200`:
```json
{
  "tools": [
    {
      "name": "search",
      "description": "Search the web",
      "inputSchema": {...}
    }
  ]
}
```

**Errors**: `403` (access denied), `404` (server not found).

### POST /api/mcp/servers/{id}/reconnect

Reconnects to an MCP server (useful after connection failures).

**Response** `200`: Server info object (same structure as list item).

**Errors**: `403` (access denied), `404` (server not found), `500` (reconnect failed).

### POST /api/mcp/servers/{id}/chat

Sends a chat message to an MCP server. The API orchestrates the LLM + tool calling loop and streams the response as AG-UI SSE events.

**Request Body**:
```json
{
  "messages": [
    {"role": "user", "content": "Search for the latest news"}
  ],
  "model_id": "gpt-4o",
  "session_id": "optional-session-uuid"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `messages` | array | yes | Conversation messages |
| `model_id` | string | no | LLM model ID to use (falls back to default chat model) |
| `session_id` | string | no | Existing session ID |

**Response**: `text/event-stream` with AG-UI events (including tool call events).

**Errors**: `400` (invalid body, no messages), `403` (access denied), `404` (server not found).

---

## MCP Session Endpoints

Authentication required. Owner-only access.

### GET /api/mcp/servers/{id}/sessions

Lists sessions for a specific MCP server.

**Response** `200`:
```json
{
  "sessions": [...]
}
```

### GET /api/mcp/servers/{id}/sessions/{sid}

Returns an MCP session with messages.

**Response** `200`: Session object.

**Errors**: `403` (not owner), `404` (not found).

### PATCH /api/mcp/servers/{id}/sessions/{sid}

Renames an MCP session.

**Request Body**:
```json
{
  "session_name": "New name"
}
```

**Response** `200`: Updated session object.

### DELETE /api/mcp/servers/{id}/sessions/{sid}

Deletes an MCP session.

**Response**: `204 No Content`

---

## Multi-MCP Endpoints

Authentication required.

### POST /api/mcp/chat

Sends a chat message using tools from multiple MCP servers simultaneously.

**Request Body**:
```json
{
  "messages": [
    {"role": "user", "content": "Search and analyze data"}
  ],
  "model_id": "gpt-4o",
  "server_ids": ["server-1", "server-2"],
  "session_id": "optional-session-uuid"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `messages` | array | yes | Conversation messages |
| `model_id` | string | no | LLM model ID |
| `server_ids` | array | yes | At least 2 MCP server IDs |
| `session_id` | string | no | Existing session ID |

**Response**: `text/event-stream` with AG-UI events. Tool call events include `serverId` to identify which server owns the tool.

**Errors**: `400` (invalid body, fewer than 2 servers), `403` (access denied), `404` (server not found).

### GET /api/mcp/sessions

Lists all multi-MCP sessions for the authenticated user.

**Response** `200`:
```json
{
  "sessions": [...]
}
```

### GET /api/mcp/sessions/{sid}

Returns a multi-MCP session with messages.

**Response** `200`: Session object.

**Errors**: `403` (not owner), `404` (not found).

### PATCH /api/mcp/sessions/{sid}

Renames a multi-MCP session.

**Request Body**:
```json
{
  "session_name": "New name"
}
```

**Response** `200`: Updated session object.

### DELETE /api/mcp/sessions/{sid}

Deletes a multi-MCP session.

**Response**: `204 No Content`

---

## Admin Agent Endpoints

Authentication required. Admin role required.

### GET /api/admin/agents

Lists all agents with full configuration (including sensitive fields).

**Response** `200`:
```json
{
  "agents": [
    {
      "id": "my-agent",
      "name": "My Agent",
      "description": "A helpful agent",
      "category": "general",
      "protocol": "rest",
      "endpoint": "https://agent.example.com",
      "agent_card_path": "",
      "forward_authorization": false,
      "require_github_token": false,
      "pipeline_final_agent": "",
      "adk_app_name": "",
      "adk_user_id": "",
      "headers": {},
      "rate_limit": null,
      "health_check": null,
      "polling": null,
      "allowed_users": ["user@example.com"],
      "allowed_groups": ["group-a"],
      "status": "healthy"
    }
  ]
}
```

### GET /api/admin/agents/{id}

Returns a single agent with full admin details.

**Response** `200`: Admin agent response object (same structure as list item).

**Errors**: `404` (not found).

### POST /api/admin/agents

Creates a new agent.

**Request Body**:
```json
{
  "id": "new-agent",
  "name": "New Agent",
  "description": "Description",
  "category": "general",
  "protocol": "rest",
  "endpoint": "https://agent.example.com",
  "agent_card_path": "",
  "forward_authorization": false,
  "require_github_token": false,
  "pipeline_final_agent": "",
  "adk_app_name": "",
  "adk_user_id": "",
  "headers": {"X-Api-Key": "secret"},
  "rate_limit": {"requests_per_minute": 60, "requests_per_hour": 1000},
  "health_check": {"enabled": true, "endpoint": "/health", "interval_seconds": 30, "timeout_seconds": 5},
  "polling": {"interval_ms": 500, "timeout_seconds": 120},
  "allowed_users": ["user@example.com"],
  "allowed_groups": ["*"]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique agent identifier |
| `name` | string | yes | Display name |
| `description` | string | no | Agent description |
| `category` | string | no | Agent category |
| `protocol` | string | yes | `"rest"`, `"a2a"`, or `"adk"` |
| `endpoint` | string | yes | Agent base URL |
| `agent_card_path` | string | no | Path to agent-card.json (A2A only) |
| `forward_authorization` | boolean | no | Forward user's JWT to agent |
| `require_github_token` | boolean | no | Require linked GitHub account |
| `pipeline_final_agent` | string | no | Final agent ID for pipeline mode |
| `adk_app_name` | string | no | ADK app name |
| `adk_user_id` | string | no | ADK user ID |
| `headers` | object | no | Static headers to send to agent |
| `rate_limit` | object | no | Rate limit configuration |
| `health_check` | object | no | Health check configuration |
| `polling` | object | no | Polling configuration (A2A only) |
| `allowed_users` | array | no | Email addresses with access |
| `allowed_groups` | array | no | Group names with access |

**Response** `201`: Admin agent response object.

**Errors**: `400` (missing required fields, invalid body).

### PUT /api/admin/agents/{id}

Updates an existing agent.

**Request Body**: Same as POST (except `id` is taken from the URL path).

**Response** `200`: Updated admin agent response object.

**Errors**: `400` (invalid body), `500` (update failed).

### DELETE /api/admin/agents/{id}

Deletes an agent.

**Response**: `204 No Content`

**Errors**: `404` (not found).

### PUT /api/admin/agents/{id}/permissions

Updates the permission lists for an agent.

**Request Body**:
```json
{
  "allowed_users": ["user@example.com"],
  "allowed_groups": ["group-a", "*"]
}
```

**Response** `200`:
```json
{
  "allowed_users": ["user@example.com"],
  "allowed_groups": ["group-a", "*"]
}
```

**Errors**: `400` (invalid body), `500` (update failed).

---

## Admin MCP Endpoints

Authentication required. Admin role required.

### GET /api/admin/mcp

Lists all MCP servers with full configuration.

**Response** `200`:
```json
{
  "servers": [
    {
      "id": "my-server",
      "name": "My MCP Server",
      "description": "Tool server",
      "transport": "sse",
      "url": "https://mcp.example.com",
      "headers": {},
      "allowed_users": [],
      "allowed_groups": ["*"],
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

### GET /api/admin/mcp/{id}

Returns a single MCP server with full details.

**Response** `200`: MCP server object.

**Errors**: `404` (not found).

### POST /api/admin/mcp

Creates a new MCP server.

**Request Body**:
```json
{
  "id": "new-server",
  "name": "New MCP Server",
  "description": "Description",
  "transport": "sse",
  "url": "https://mcp.example.com/sse",
  "headers": {"Authorization": "Bearer token"},
  "allowed_users": [],
  "allowed_groups": ["*"]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique server identifier |
| `name` | string | yes | Display name |
| `description` | string | no | Server description |
| `transport` | string | yes | Transport type (e.g., `"sse"`) |
| `url` | string | yes | Server URL |
| `headers` | object | no | Static headers for connection |
| `allowed_users` | array | no | Email addresses with access |
| `allowed_groups` | array | no | Group names with access |

**Response** `201`: MCP server object.

**Errors**: `400` (missing required fields).

### PUT /api/admin/mcp/{id}

Updates an existing MCP server.

**Request Body**: Same as POST (except `id` is taken from the URL path).

**Response** `200`: Updated MCP server object.

**Errors**: `400` (invalid body), `500` (update failed).

### DELETE /api/admin/mcp/{id}

Deletes an MCP server.

**Response**: `204 No Content`

**Errors**: `404` (not found).

### PUT /api/admin/mcp/{id}/permissions

Updates permission lists for an MCP server.

**Request Body**:
```json
{
  "allowed_users": ["user@example.com"],
  "allowed_groups": ["*"]
}
```

**Response** `200`:
```json
{
  "allowed_users": ["user@example.com"],
  "allowed_groups": ["*"]
}
```

**Errors**: `400` (invalid body), `500` (update failed).

---

## Admin LLM Endpoints

Authentication required. Admin role required.

### GET /api/admin/llm

Lists all LLM models. API keys are masked in the response (first 4 + `****` + last 4 characters).

**Response** `200`:
```json
{
  "models": [
    {
      "id": "gpt-4o",
      "name": "GPT-4o",
      "provider": "openai",
      "model": "gpt-4o",
      "api_key": "sk-p****abcd",
      "role": "chat",
      "enabled": true,
      "is_default": true
    }
  ]
}
```

### GET /api/admin/llm/{id}

Returns a single LLM model (API key masked).

**Response** `200`: LLM model object.

**Errors**: `404` (not found).

### POST /api/admin/llm

Creates a new LLM model.

**Request Body**:
```json
{
  "id": "gpt-4o",
  "name": "GPT-4o",
  "provider": "openai",
  "model": "gpt-4o",
  "api_key": "sk-...",
  "role": "chat",
  "enabled": true,
  "is_default": true
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique model identifier |
| `name` | string | yes | Display name |
| `provider` | string | yes | LLM provider (e.g., `"openai"`, `"anthropic"`) |
| `model` | string | yes | Provider model name |
| `api_key` | string | yes | API key for the provider |
| `role` | string | no | Model role: `"chat"` (default), `"summarizer"`, or `"file_processor"` |
| `enabled` | boolean | no | Whether the model is active |
| `is_default` | boolean | no | Whether this is the default model for its role |

**Response** `201`: LLM model object (API key masked).

**Errors**: `400` (missing required fields).

### PUT /api/admin/llm/{id}

Updates an existing LLM model. If `api_key` is omitted or contains a masked value, the existing key is preserved.

**Request Body**: Same as POST (except `id` is taken from the URL path).

**Response** `200`: Updated LLM model object (API key masked).

**Errors**: `400` (invalid body), `404` (not found), `500` (update failed).

### DELETE /api/admin/llm/{id}

Deletes an LLM model.

**Response**: `204 No Content`

**Errors**: `404` (not found).

---

## Admin User Endpoints

Authentication required. Admin role required.

### GET /api/admin/users

Lists all users in the system.

**Response** `200`:
```json
{
  "users": [
    {
      "id": "uuid",
      "email": "user@example.com",
      "role": "user",
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

### PUT /api/admin/users/{email}/role

Updates a user's role.

**Request Body**:
```json
{
  "role": "admin"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `role` | string | yes | `"admin"` or `"user"` |

**Response** `200`:
```json
{
  "email": "user@example.com",
  "role": "admin"
}
```

**Errors**: `400` (invalid role), `404` (user not found).
