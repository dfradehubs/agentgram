# Agentgram Protocols Overview

Agentgram supports three protocols for connecting to remote agents. All protocols are converted to AG-UI events for the frontend.

## Protocol Comparison

| Feature             | Custom                  | A2A (Google Standard)    | ADK (Google REST SSE)    |
|---------------------|------------------------|--------------------------|--------------------------|
| **Standard**        | Agentgram custom       | a2a-protocol.org         | Google ADK framework     |
| **Transport**       | HTTP POST + SSE        | JSON-RPC 2.0 + SSE      | HTTP POST + SSE          |
| **Request format**  | `{query, conversation_id}` | JSON-RPC `message/stream` | `{appName, userId, sessionId, newMessage}` |
| **Response format** | Named SSE events       | JSON-RPC wrapped events  | Plain JSON events        |
| **Tool calls**      | `tool_start/input/result` events | Via `Part.data` (structured) | `functionCall/functionResponse` |
| **Sessions**        | `session_id` in events | `contextId` in messages  | `sessionId` in request   |
| **Agent discovery** | N/A                    | Agent card (`.well-known/agent.json`) | N/A             |
| **Pipeline support**| N/A                    | `metadata.adk_author`   | `event.author`           |
| **Config key**      | `protocol: "custom"`   | `protocol: "a2a"`        | `protocol: "adk"`        |

## When to Use Each Protocol

### Custom
- Simple agents with basic text streaming
- Agents that already implement the Agentgram SSE format
- Legacy agents with `start/chunk/end` event patterns
- Agents that respond with JSON (non-streaming)

### A2A (Google Standard)
- Agents built with `a2a-go` or other A2A-compliant frameworks
- Multi-agent pipelines with state transitions
- Agents that need structured data exchange (data parts, file parts)
- Agents requiring formal agent discovery (agent cards)

### ADK (Google REST SSE)
- Agents built with Google's ADK framework (`adkrest.NewHandler()`)
- Agents with native function calling (tool use)
- Google Cloud-based agent deployments
- Agents requiring explicit session and user management

## Architecture

```
                                    ┌───────────────────────┐
                                    │   REST Agent          │
                              ┌────>│   SSE: event/data     │
                              │     └───────────────────────┘
┌──────────┐   AG-UI    ┌────┴────┐
│ Frontend │ ◄────────► │ Backend │  ┌───────────────────────┐
│ (Next.js)│   SSE      │ (Go    │  │   A2A Agent           │
└──────────┘            │  Proxy) ├─>│   JSON-RPC + SSE      │
                        └────┬────┘  └───────────────────────┘
                              │
                              │     ┌───────────────────────┐
                              └────>│   ADK Agent           │
                                    │   POST /run_sse       │
                                    └───────────────────────┘
```

## Documentation

- [REST Agent Contract](./AGENT_REST_CONTRACT.md)
- [A2A Agent Contract](./AGENT_A2A_CONTRACT.md)
- [ADK Agent Contract](./AGENT_ADK_CONTRACT.md)
