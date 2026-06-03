# REST Agent API Contract (Custom)

This document defines the custom REST SSE event contract for agents integrating with Agentgram. This is Agentgram's original protocol for simple text streaming and tool calls.

> **Note**: Agentgram also supports [A2A (Google Standard)](./AGENT_A2A_CONTRACT.md) and [ADK REST SSE](./AGENT_ADK_CONTRACT.md) protocols. See [Protocols Overview](./PROTOCOLS_OVERVIEW.md) for a comparison.

## Overview

Agentgram connects to external agents via HTTP POST. The agent responds with an SSE stream (`text/event-stream`) that the backend proxy converts to AG-UI events for the frontend.

## Request Format

The backend sends a POST request to the agent's configured URL:

```
POST <agent_url>
Content-Type: application/json
Authorization: <forwarded from client if present>
```

### Request Body

```json
{
  "query": "The user's message",
  "conversation_id": "optional-session-uuid"
}
```

| Field             | Type   | Required | Description                                      |
|-------------------|--------|----------|--------------------------------------------------|
| `query`           | string | yes      | The user's message (or concatenated messages)     |
| `conversation_id` | string | no       | Session ID for conversation continuity            |

## Response Format

The agent must respond with:
- **Status**: `200 OK`
- **Content-Type**: `text/event-stream`

### SSE Event Types

Events use the standard SSE format with named events:

```
event: <event_type>
data: <json_payload>

```

> **Important**: Each event block must end with a blank line (`\n\n`).

---

### Session Start (optional)

Emitted once at the beginning to establish the session.

```
event: session_id
data: {"session_id": "uuid-of-session"}
```

| Field        | Type   | Description                     |
|--------------|--------|---------------------------------|
| `session_id` | string | Agent-managed session identifier |

---

### Text Streaming

Text content is streamed as `content_delta` events:

```
event: content_delta
data: {"text": "Hello, "}

event: content_delta
data: {"text": "how can I help?"}
```

| Field  | Type   | Description          |
|--------|--------|----------------------|
| `text` | string | Text chunk to append |

---

### Tool Calls

Tool calls are represented by three event types that must appear in order:

#### 1. tool_start

Signals a new tool invocation.

```
event: tool_start
data: {"tool_id": "toolu_abc123", "tool_name": "SearchTool"}
```

| Field       | Type   | Required | Description                    |
|-------------|--------|----------|--------------------------------|
| `tool_id`   | string | yes      | Unique identifier for this call |
| `tool_name` | string | yes      | Name of the tool being called  |
| `message`   | string | no       | Human-readable status message  |

#### 2. tool_input

Contains the arguments passed to the tool.

```
event: tool_input
data: {"tool_id": "toolu_abc123", "tool_name": "SearchTool", "args": {"query": "nginx logs"}}
```

| Field       | Type   | Required | Description                    |
|-------------|--------|----------|--------------------------------|
| `tool_id`   | string | yes      | Must match the `tool_start`    |
| `tool_name` | string | no       | Tool name (redundant but useful)|
| `args`      | object | yes      | Arguments as a JSON object     |

#### 3. tool_result

Contains the result of the tool execution.

```
event: tool_result
data: {"tool_id": "toolu_abc123", "tool_name": "SearchTool", "result": "Found 42 matches", "is_error": false}
```

The backend extracts the result text using the following priority:

1. `result` (string) - direct result text
2. `output` (string) - alternative field name
3. `content` (string) - alternative field name
4. `response.content[].text` (Anthropic/ADK style) - nested content blocks with full data
5. `message` (string) - human-readable summary (lower priority, often a short summary)
6. JSON-marshaled fallback of any of the above fields if they are objects

| Field       | Type           | Required | Description                         |
|-------------|----------------|----------|-------------------------------------|
| `tool_id`   | string         | yes      | Must match the `tool_start`         |
| `tool_name` | string         | no       | Tool name                           |
| `result`    | string/object  | no*      | Tool execution result               |
| `output`    | string/object  | no*      | Alternative to `result`             |
| `content`   | string/object  | no*      | Alternative to `result`             |
| `message`   | string         | no*      | Human-readable summary              |
| `response`  | object         | no*      | Anthropic-style `{content: [{text}]}`|
| `is_error`  | boolean        | no       | Whether the tool execution failed   |
| `duration`  | number         | no       | Execution time in seconds           |

> \* At least one result field should be provided.

---

### Stream End

Signal the end of the response:

```
event: end
data: {"session_id": "uuid-of-session"}
```

| Field        | Type   | Required | Description                     |
|--------------|--------|----------|---------------------------------|
| `session_id` | string | no       | Session ID for next request     |

---

## Complete Example

```
event: session_id
data: {"session_id": "abc-123"}

event: content_delta
data: {"text": "Let me search for that. "}

event: tool_start
data: {"tool_id": "call_1", "tool_name": "SearchLogs", "message": "Searching logs..."}

event: tool_input
data: {"tool_id": "call_1", "tool_name": "SearchLogs", "args": {"query": "error 500", "index": "prod-api"}}

event: tool_result
data: {"tool_id": "call_1", "tool_name": "SearchLogs", "result": "Found 15 errors in the last hour", "is_error": false, "duration": 1.2}

event: content_delta
data: {"text": "I found 15 errors in the last hour. "}

event: content_delta
data: {"text": "Here's a summary..."}

event: end
data: {"session_id": "abc-123"}
```

## Alternative Formats

The proxy also supports these legacy/alternative event formats:

### JSON `type` field instead of SSE event name

If the agent cannot set SSE event names, it can include a `type` field in the JSON data:

```
data: {"type": "content_delta", "text": "Hello"}
data: {"type": "tool_start", "tool_id": "call_1", "tool_name": "Search"}
```

### `start`/`chunk`/`end` events

Legacy format for simple text streaming:

```
event: start
data: {"session_id": "abc-123"}

event: chunk
data: {"content": "Hello, how can I help?"}

event: end
data: {"session_id": "abc-123"}
```

### Non-SSE JSON Response

If the agent responds with `Content-Type: application/json` instead of SSE, the backend will extract content from:

1. OpenAI format: `choices[0].message.content`
2. Anthropic format: `content[0].text`
3. Simple fields: `response`, `text`, or `content`
4. Full JSON body as fallback

## Sessions API (optional)

If the agent manages sessions, it should expose these endpoints:

```
GET    /api/sessions                  → List sessions
GET    /api/sessions/:sessionId       → Get session with messages
PATCH  /api/sessions/:sessionId       → Rename session {"title": "new name"}
DELETE /api/sessions/:sessionId       → Delete session
```

The backend proxies these requests from `GET /api/agents/:agentId/sessions` to the agent.

### List Sessions Response

```json
{
  "sessions": [
    {
      "id": "uuid",
      "title": "Session title",
      "created_at": "2025-01-15T10:30:00Z",
      "updated_at": "2025-01-15T11:00:00Z"
    }
  ]
}
```

### Get Session Response

```json
{
  "id": "uuid",
  "title": "Session title",
  "messages": [
    {"role": "user", "content": "Hello"},
    {"role": "assistant", "content": "Hi! How can I help?"}
  ],
  "created_at": "2025-01-15T10:30:00Z",
  "updated_at": "2025-01-15T11:00:00Z"
}
```
