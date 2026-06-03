# ADK Agent Protocol Contract

This document defines the ADK (Agent Development Kit) REST SSE protocol contract for agents integrating with Agentgram, based on Google's ADK REST API.

## Overview

ADK is Google's framework for building AI agents. The ADK REST SSE protocol uses a simple HTTP POST with SSE response, where each event is a plain JSON object (no JSON-RPC wrapper). Agentgram converts ADK events to AG-UI protocol for the frontend.

## Configuration

```yaml
- id: "kube-agent"
  name: "Kubernetes Agent"
  protocol: "adk"
  endpoint: "http://agent-host:8080/agent/run_sse"
  adk_app_name: "kube_agent"     # default: agent ID
  adk_user_id: "agentgram"       # default: "agentgram"
  pipeline_final_agent: "final"  # optional: for multi-agent pipelines
```

| Field          | Type   | Default       | Description                          |
|----------------|--------|---------------|--------------------------------------|
| `adk_app_name` | string | agent `id`    | Application name sent in requests    |
| `adk_user_id`  | string | `"agentgram"` | Default user ID for requests         |

## Request Format

```
POST <agent_endpoint>
Content-Type: application/json
Accept: text/event-stream
```

```json
{
  "appName": "kube_agent",
  "userId": "agentgram",
  "sessionId": "optional-session-uuid",
  "newMessage": {
    "role": "user",
    "parts": [
      {"text": "List all pods in the default namespace"}
    ]
  },
  "streaming": true
}
```

| Field        | Type    | Required | Description                              |
|--------------|---------|----------|------------------------------------------|
| `appName`    | string  | yes      | Application/agent name                   |
| `userId`     | string  | yes      | User identifier                          |
| `sessionId`  | string  | no       | Session ID for conversation continuity   |
| `newMessage` | Content | yes      | The user's message                       |
| `streaming`  | boolean | no       | Request streaming response (default true)|

## Response Format (SSE Stream)

The agent responds with `Content-Type: text/event-stream`. Each SSE event contains a plain JSON object:

```
data: {"content":{"role":"model","parts":[{"text":"Here are the pods..."}]},"author":"agent_name","invocationId":"inv-123"}
```

### Event Structure

```json
{
  "content": {
    "role": "model",
    "parts": [
      {"text": "Response text"},
      {"functionCall": {"name": "list_pods", "args": {"namespace": "default"}}},
      {"functionResponse": {"name": "list_pods", "response": {"pods": [...]}}}
    ]
  },
  "author": "agent_name",
  "invocationId": "inv-123",
  "actions": {
    "stateDelta": {},
    "transferToAgent": "other_agent"
  },
  "timestamp": 1234567890.123,
  "id": "event-id"
}
```

### Part Types

| Part Field       | Description                          |
|------------------|--------------------------------------|
| `text`           | Plain text content                   |
| `functionCall`   | Tool invocation (name + args)        |
| `functionResponse` | Tool result (name + response)     |
| `inlineData`     | Binary data (mimeType + base64 data) |

## ADK -> AG-UI Mapping

| ADK Event Part            | AG-UI Event                                      |
|---------------------------|--------------------------------------------------|
| `text` (final agent)      | TEXT_MESSAGE_START + TEXT_MESSAGE_CONTENT          |
| `text` (intermediate)     | TEXT_MESSAGE_START (isThinking) + TEXT_MESSAGE_CONTENT + TEXT_MESSAGE_END |
| `functionCall`            | TOOL_CALL_START + TOOL_CALL_ARGS                  |
| `functionResponse`        | TOOL_CALL_END                                     |
| Stream end                | TEXT_MESSAGE_END + RUN_FINISHED                    |

### Pipeline Agents

When `pipeline_final_agent` is configured, text from agents whose `author` field doesn't match is emitted as thinking steps (isThinking=true).

## Complete Example

```
data: {"content":{"role":"model","parts":[{"text":"Let me check the pods for you."}]},"author":"kube_agent","invocationId":"inv-1"}

data: {"content":{"role":"model","parts":[{"functionCall":{"name":"kubectl_get","args":{"resource":"pods","namespace":"default"}}}]},"author":"kube_agent","invocationId":"inv-1"}

data: {"content":{"role":"model","parts":[{"functionResponse":{"name":"kubectl_get","response":{"pods":["nginx-abc","redis-xyz"]}}}]},"author":"kube_agent","invocationId":"inv-2"}

data: {"content":{"role":"model","parts":[{"text":"Found 2 pods in the default namespace:\n- nginx-abc\n- redis-xyz"}]},"author":"kube_agent","invocationId":"inv-3"}
```

## Sessions

ADK agents manage their own sessions. The `sessionId` field in the request establishes session continuity. If omitted, the agent creates a new session.

## See Also

- [REST Agent Contract](./AGENT_REST_CONTRACT.md) - Custom REST SSE protocol
- [A2A Agent Contract](./AGENT_A2A_CONTRACT.md) - Google A2A JSON-RPC protocol
- [Protocols Overview](./PROTOCOLS_OVERVIEW.md) - Comparison of all protocols
