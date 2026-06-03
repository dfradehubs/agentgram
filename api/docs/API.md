# API Specification - Agentgram API

## Overview

This document describes the Agentgram API for web developers.

**Base URL**: `http://localhost:8080` (development)

## Authentication

All requests (except health) require JWT authentication:

```
Authorization: Bearer <token>
```

The JWT must be valid from Keycloak with Google Workspace groups.

## Endpoints

### Health Checks

#### Liveness

```
GET /health
```

**Response 200**:
```json
{
  "status": "ok"
}
```

#### Readiness

```
GET /health/ready
```

**Response 200**:
```json
{
  "status": "ready",
  "details": {
    "total_agents": 4,
    "healthy_agents": 3
  }
}
```

---

### Agents

#### List Agents

**IMPORTANT**: This endpoint returns ONLY the agents the user has access to based on their JWT groups and email.

```
GET /api/agents
Authorization: Bearer <token>
```

**Response 200**:
```json
{
  "agents": [
    {
      "id": "agent-sales",
      "name": "Sales Assistant",
      "description": "Sales assistant",
      "category": "sales",
      "protocol": "custom",
      "status": "healthy",
      "metadata": {
        "icon": "shopping-cart",
        "color": "#4CAF50"
      }
    },
    {
      "id": "logs-agent",
      "name": "Logs Agent",
      "description": "OpenSearch log analysis",
      "category": "observability",
      "protocol": "custom",
      "status": "healthy",
      "metadata": {}
    }
  ]
}
```

**Response 401**:
```json
{
  "error": "invalid token"
}
```

**Note**: Filtering is done automatically in the API. If the JWT has `groups: ["google-workspace/sre@example.com"]`, the user will only see agents that have that group in `allowed_groups` or their email in `allowed_users`.

#### Get Agent Details

```
GET /api/agents/{agentId}
Authorization: Bearer <token>
```

**Response 200**:
```json
{
  "id": "agent-sales",
  "name": "Sales Assistant",
  "description": "Sales assistant",
  "category": "sales",
  "protocol": "custom",
  "status": "healthy",
  "metadata": {
    "icon": "shopping-cart",
    "color": "#4CAF50"
  }
}
```

**Response 403**:
```json
{
  "error": "access denied"
}
```

**Response 404**:
```json
{
  "error": "agent not found"
}
```

#### Get Agent Health

```
GET /api/agents/{agentId}/health
Authorization: Bearer <token>
```

**Response 200**:
```json
{
  "status": "healthy",
  "details": {
    "agent_id": "agent-sales",
    "name": "Sales Assistant",
    "protocol": "custom"
  }
}
```

---

### Chat

#### Chat with Agent

The chat endpoint **always responds with SSE** (Server-Sent Events) using the **AG-UI protocol**, regardless of the agent's protocol (Custom, A2A, or ADK).

```
POST /api/agents/{agentId}/chat
Authorization: Bearer <token>
Content-Type: application/json
```

**Request Body**:
```json
{
  "messages": [
    {"role": "user", "content": "Hello, I need help"}
  ],
  "session_id": "optional-session-uuid"
}
```

**Response (SSE with AG-UI events)**:

```
Content-Type: text/event-stream
Cache-Control: no-cache

data: {"type":"RUN_STARTED","threadId":"...","runId":"..."}

data: {"type":"TEXT_MESSAGE_START","messageId":"...","role":"assistant"}

data: {"type":"TEXT_MESSAGE_CONTENT","messageId":"...","delta":"Hello"}

data: {"type":"TEXT_MESSAGE_CONTENT","messageId":"...","delta":", how can I help?"}

data: {"type":"TEXT_MESSAGE_END","messageId":"..."}

data: {"type":"RUN_FINISHED","threadId":"...","runId":"..."}
```

#### AG-UI Event Types

| Type | Description | Key Fields |
|------|-------------|------------|
| `RUN_STARTED` | Run lifecycle start | `threadId`, `runId` |
| `RUN_FINISHED` | Run lifecycle end | `threadId`, `runId` |
| `RUN_ERROR` | Error during run | `message` |
| `TEXT_MESSAGE_START` | Start of assistant message | `messageId`, `role` |
| `TEXT_MESSAGE_CONTENT` | Streaming text chunk | `messageId`, `delta` |
| `TEXT_MESSAGE_END` | End of assistant message | `messageId` |
| `TOOL_CALL_START` | Start of tool call | `toolCallId`, `toolCallName` |
| `TOOL_CALL_ARGS` | Tool call arguments (streaming) | `toolCallId`, `delta` |
| `TOOL_CALL_END` | End of tool call | `toolCallId` |
| `CUSTOM` | Custom event passthrough | varies |

#### HTTP Errors

| Status | Response |
|--------|-----------|
| 401 | `{"error": "invalid token"}` |
| 403 | `{"error": "access denied"}` |
| 404 | `{"error": "agent not found"}` |
| 502 | `{"error": "agent unavailable"}` |

---

### Sessions

Sessions are managed by the API and stored in Redis.

#### List Sessions

```
GET /api/agents/{agentId}/sessions
Authorization: Bearer <token>
```

**Response 200**:
```json
{
  "sessions": [
    {
      "session_id": "uuid",
      "session_name": "My conversation",
      "created_at": 1704067200,
      "last_activity": 1704067200,
      "message_count": 5
    }
  ]
}
```

#### Get Session

```
GET /api/agents/{agentId}/sessions/{sessionId}
Authorization: Bearer <token>
```

**Response 200**:
```json
{
  "session_id": "uuid",
  "session_name": "My conversation",
  "user_id": "user@example.com",
  "created_at": 1704067200,
  "last_activity": 1704067200,
  "message_count": 5,
  "messages": [
    {"role": "user", "content": "Hello"},
    {"role": "assistant", "content": "Hi!"}
  ]
}
```

#### Rename Session

```
PATCH /api/agents/{agentId}/sessions/{sessionId}
Authorization: Bearer <token>
Content-Type: application/json

{
  "session_name": "New name"
}
```

#### Delete Session

```
DELETE /api/agents/{agentId}/sessions/{sessionId}
Authorization: Bearer <token>
```

---

### Read State

Read state tracks which messages a user has read per session, enabling unread badges across devices.

#### Get Read State

```
GET /api/read-state
Authorization: Bearer <token>
```

**Response 200**:
```json
{
  "session-uuid-1": 5,
  "session-uuid-2": 12
}
```

#### Mark Session as Read

```
PUT /api/read-state/{sessionId}
Authorization: Bearer <token>
Content-Type: application/json

{
  "count": 15
}
```

**Response 204**: No content

#### Batch Update (Migration)

Used to migrate read state from localStorage to server.

```
PUT /api/read-state
Authorization: Bearer <token>
Content-Type: application/json

{
  "session-uuid-1": 5,
  "session-uuid-2": 12
}
```

**Response 204**: No content

---

## Web Implementation Example

### JavaScript/TypeScript

```javascript
async function chatWithAgent(agentId, messages, onEvent) {
  const response = await fetch(`/api/agents/${agentId}/chat`, {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({ messages })
  });

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || `HTTP ${response.status}`);
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split('\n');
    buffer = lines.pop() || '';

    for (const line of lines) {
      if (line.startsWith('data: ')) {
        try {
          const event = JSON.parse(line.slice(6));

          switch (event.type) {
            case 'RUN_STARTED':
              console.log('Run started:', event.runId);
              break;
            case 'TEXT_MESSAGE_START':
              console.log('Message started:', event.messageId);
              break;
            case 'TEXT_MESSAGE_CONTENT':
              onEvent(event.delta);
              break;
            case 'TEXT_MESSAGE_END':
              console.log('Message ended:', event.messageId);
              break;
            case 'RUN_FINISHED':
              console.log('Run finished:', event.runId);
              break;
            case 'RUN_ERROR':
              throw new Error(event.message);
          }
        } catch (e) {
          console.error('Failed to parse SSE:', e);
        }
      }
    }
  }
}

// Usage
await chatWithAgent(
  'agent-sales',
  [{ role: 'user', content: 'Hello!' }],
  (delta) => console.log('Delta:', delta)
);
```

### React Hook

```typescript
import { useState, useCallback } from 'react';

interface Message {
  role: 'user' | 'assistant';
  content: string;
}

export function useAgentChat(agentId: string, token: string) {
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const sendMessage = useCallback(async (
    messages: Message[],
    onDelta: (delta: string) => void
  ) => {
    setIsLoading(true);
    setError(null);

    try {
      const response = await fetch(`/api/agents/${agentId}/chat`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ messages })
      });

      if (!response.ok) {
        const err = await response.json();
        throw new Error(err.error);
      }

      const reader = response.body!.getReader();
      const decoder = new TextDecoder();

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        const lines = decoder.decode(value).split('\n');
        for (const line of lines) {
          if (line.startsWith('data: ')) {
            const event = JSON.parse(line.slice(6));
            if (event.type === 'TEXT_MESSAGE_CONTENT') {
              onDelta(event.delta);
            } else if (event.type === 'RUN_ERROR') {
              throw new Error(event.message);
            }
          }
        }
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Unknown error');
    } finally {
      setIsLoading(false);
    }
  }, [agentId, token]);

  return { sendMessage, isLoading, error };
}
```

---

## Important Notes

1. **AG-UI Protocol**: The API always responds with AG-UI events via SSE, regardless of whether the agent uses Custom, A2A, or ADK protocol.

2. **Permission Filtering**: The `/api/agents` endpoint already filters by permissions. The web doesn't need to implement permission logic.

3. **Error Handling**: Errors during streaming are sent as AG-UI `RUN_ERROR` events. Agent errors are sanitized (no internal details leaked).

4. **Timeout**: SSE connections can last several minutes for A2A agents with polling.

5. **Reconnection**: The web should implement reconnection logic if the connection is lost.
