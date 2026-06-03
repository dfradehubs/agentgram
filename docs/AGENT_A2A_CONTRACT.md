# A2A Agent Protocol Contract

This document defines the A2A (Agent-to-Agent) protocol contract for agents integrating with Agentgram, based on the [Google A2A standard](https://a2a-protocol.org).

## Overview

A2A is a JSON-RPC 2.0 based protocol for agent-to-agent communication. Agentgram connects to A2A agents via `message/stream` and converts streaming events to AG-UI protocol for the frontend.

## Configuration

```yaml
- id: "my-a2a-agent"
  name: "My A2A Agent"
  protocol: "a2a"
  endpoint: "http://agent-host:8080/a2a/agent"
  agent_card_path: "/.well-known/agent.json"  # default
  pipeline_final_agent: "summary"              # optional: for multi-agent pipelines
```

## Agent Card Discovery

The backend fetches the agent card at `{endpoint}{agent_card_path}`:

```
GET /.well-known/agent.json
```

Response:

```json
{
  "name": "My Agent",
  "description": "Agent description",
  "url": "http://agent-host:8080",
  "version": "1.0.0",
  "capabilities": {
    "streaming": true,
    "pushNotifications": false
  },
  "skills": [
    {"id": "search", "name": "Search", "description": "Search capability"}
  ],
  "defaultInputModes": ["text"],
  "defaultOutputModes": ["text"],
  "preferredTransport": "jsonrpc"
}
```

## Request Format

JSON-RPC 2.0 request to the agent endpoint:

```
POST <agent_endpoint>
Content-Type: application/json
Accept: text/event-stream
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "message/stream",
  "params": {
    "message": {
      "messageId": "uuid",
      "role": "user",
      "parts": [
        {"kind": "text", "text": "Hello, agent!"}
      ],
      "contextId": "optional-session-uuid"
    }
  }
}
```

## Response Format (SSE Stream)

The agent responds with `Content-Type: text/event-stream`. Each SSE event contains a JSON-RPC response wrapper:

```
data: {"jsonrpc":"2.0","id":1,"result":{...event...}}
```

### Event Types

Events are discriminated by `result.kind`:

#### status-update

Reports task state transitions.

```json
{
  "kind": "status-update",
  "taskId": "task-uuid",
  "contextId": "session-uuid",
  "status": {
    "state": "working",
    "message": {
      "messageId": "msg-uuid",
      "role": "agent",
      "parts": [{"kind": "text", "text": "Processing..."}]
    }
  },
  "final": false
}
```

**States**: `submitted`, `working`, `input-required`, `completed`, `failed`, `canceled`, `rejected`, `auth-required`

#### artifact-update

Delivers output artifacts (text, files, structured data).

```json
{
  "kind": "artifact-update",
  "artifact": {
    "artifactId": "artifact-uuid",
    "name": "response",
    "parts": [{"kind": "text", "text": "Here is the answer..."}],
    "append": false,
    "lastChunk": true
  }
}
```

### Part Types

| Kind   | Fields                     | Description              |
|--------|----------------------------|--------------------------|
| `text` | `text`, `metadata`         | Text content             |
| `file` | `file.name`, `file.bytes`, `file.uri` | File content  |
| `data` | `data` (map)               | Structured JSON data     |

## A2A -> AG-UI Mapping

| A2A Event                              | AG-UI Event              |
|----------------------------------------|--------------------------|
| status-update (state: working, text)   | TEXT_MESSAGE_START (isThinking) + TEXT_MESSAGE_CONTENT + TEXT_MESSAGE_END |
| status-update (state: completed, text) | TEXT_MESSAGE_CONTENT + TEXT_MESSAGE_END + RUN_FINISHED |
| status-update (state: failed)          | RUN_ERROR                |
| status-update (state: rejected)        | RUN_ERROR                |
| status-update (state: auth-required)   | RUN_ERROR                |
| status-update (state: canceled)        | RUN_FINISHED             |
| artifact-update (text part)            | TEXT_MESSAGE_CONTENT      |
| artifact-update (data part)            | TEXT_MESSAGE_START (isThinking) + TEXT_MESSAGE_CONTENT |

### Pipeline Agents

When `pipeline_final_agent` is configured, text from non-final agents is emitted as thinking steps (isThinking=true). The `metadata.adk_author` field identifies which agent in the pipeline produced the artifact.

## Complete Example

```
data: {"jsonrpc":"2.0","id":1,"result":{"kind":"status-update","taskId":"t1","contextId":"ctx1","status":{"state":"working","message":{"messageId":"m1","role":"agent","parts":[{"kind":"text","text":"Analyzing your request..."}]}},"final":false}}

data: {"jsonrpc":"2.0","id":1,"result":{"kind":"artifact-update","artifact":{"artifactId":"a1","parts":[{"kind":"text","text":"Here is your answer: ..."}],"lastChunk":true}}}

data: {"jsonrpc":"2.0","id":1,"result":{"kind":"status-update","taskId":"t1","contextId":"ctx1","status":{"state":"completed"},"final":true}}
```

## Sessions

A2A uses `contextId` for session continuity. The backend maps the `contextId` from the agent response to `AgentSessionID` in the session store.

## See Also

- [REST Agent Contract](./AGENT_REST_CONTRACT.md) - Custom REST SSE protocol
- [ADK Agent Contract](./AGENT_ADK_CONTRACT.md) - Google ADK REST SSE protocol
- [Protocols Overview](./PROTOCOLS_OVERVIEW.md) - Comparison of all protocols
