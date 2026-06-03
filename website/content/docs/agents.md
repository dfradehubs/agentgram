---
title: Agents & protocols
weight: 5
---

Agents are managed at runtime from the built-in **admin panel** (`/admin`) and stored in PostgreSQL —
no redeploy is needed to add one. For each agent you choose its protocol, its endpoint, and who can
reach it.

## Supported protocols

Agentgram speaks three protocols on the way in and converts all of them to AG-UI events on the way
out:

| Protocol | Transport | Contract |
| -------- | --------- | -------- |
| **Custom** | REST endpoint returning SSE or JSON | [REST contract](https://github.com/dfradehubs/agentgram/blob/main/docs/AGENT_REST_CONTRACT.md) |
| **A2A** | JSON-RPC (Agent-to-Agent) | [A2A contract](https://github.com/dfradehubs/agentgram/blob/main/docs/AGENT_A2A_CONTRACT.md) |
| **ADK** | Google ADK over REST/SSE | [ADK contract](https://github.com/dfradehubs/agentgram/blob/main/docs/AGENT_ADK_CONTRACT.md) |

See the [protocols overview](https://github.com/dfradehubs/agentgram/blob/main/docs/PROTOCOLS_OVERVIEW.md)
for how each one is mapped onto AG-UI.

## Permissions

Access is controlled per agent. An agent can be reachable by:

- **Groups** — e.g. Google Workspace groups (`google-workspace/team@example.com`), with `*` for public access.
- **Individual users** — by email.

When a request comes in, the API filters the visible agents by the caller's identity (from the JWT
claims) before routing.

## The chat contract

A chat request is a list of messages plus an optional session id:

```http
POST /api/agents/{agentId}/chat
Content-Type: application/json
Authorization: Bearer <jwt>      # only when auth is enabled

{
  "messages": [
    { "role": "user", "content": "Hello" }
  ],
  "session_id": "optional-uuid"
}
```

The response is a stream of AG-UI events over SSE — the same shape regardless of the agent's protocol:

```text
data: {"type":"RUN_STARTED","threadId":"...","runId":"..."}
data: {"type":"TEXT_MESSAGE_START","messageId":"...","role":"assistant"}
data: {"type":"TEXT_MESSAGE_CONTENT","messageId":"...","delta":"Hello"}
data: {"type":"TEXT_MESSAGE_END","messageId":"..."}
data: {"type":"RUN_FINISHED","threadId":"...","runId":"..."}
```

Full endpoint details are in the [API Reference](/agentgram/api/).
