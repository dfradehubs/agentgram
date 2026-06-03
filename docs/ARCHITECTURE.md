# Agent Architecture in Agentgram

## Overview

Agentgram connects to remote agents through 3 protocols, allows the creation of internal agents (custom agents) with LLM + tools, and orchestrates multi-agent sessions with two execution modes.

```
┌─────────────┐     ┌──────────────────────────────────────┐     ┌─────────────────┐
│             │     │              API (Go)                │     │  Agent (REST)   │
│    Web      │────>│                                      │────>│  SSE/JSON       │
│  (Next.js)  │<────│  ┌──────────┐  ┌──────────────────┐ │<────└─────────────────┘
│             │     │  │ Registry │  │  Proxy (router)   │ │     ┌─────────────────┐
│  AG-UI SSE  │     │  │ (DB+cache│  │  ├─ RESTProxy    │ │────>│  Agent (A2A)    │
│             │     │  │  30s)    │  │  ├─ A2AProxy     │ │<────│  JSON-RPC       │
└─────────────┘     │  └──────────┘  │  └─ ADKProxy     │ │     └─────────────────┘
                    │                └──────────────────┘ │     ┌─────────────────┐
                    │  ┌──────────────────┐               │────>│  Agent (ADK)    │
                    │  │ Custom Agent     │               │<────│  Google Vertex   │
                    │  │ Runtime (LLM +   │               │     └─────────────────┘
                    │  │ tool-call loop)  │               │     ┌─────────────────┐
                    │  └──────────────────┘               │────>│  MCP Servers    │
                    └──────────────────────────────────────┘     └─────────────────┘
```

---

## 1. External Agents (3 protocols)

Agents are defined in PostgreSQL and cached in memory (`agents/registry.go`, auto-refresh every 30s). Each agent has a `Protocol` that determines the proxy.

### 1.1 Custom (REST) — `proxy/rest.go`

- **Request**: `POST` to the agent endpoint with `{"query": "...", "conversation_id": "..."}`
- **Response**: Auto-detects `text/event-stream` (SSE) or JSON
  - SSE: parses native AG-UI (`TEXT_MESSAGE_CONTENT`), legacy (`content_delta`, `chunk`), and tool calls
  - JSON: extracts content from OpenAI, Anthropic, or simple formats
- **Sessions**: The agent returns `session_id` or `conversation_id` in the SSE stream, and the API maps it

### 1.2 A2A (Agent-to-Agent) — `proxy/a2a.go`

- **Request**: JSON-RPC 2.0 `message/stream` with parts (`text`, `data`, `file`)
- **Response**: SSE with JSON-RPC events
  - States: `submitted` → `working` → `completed`/`failed`/`rejected`/`auth-required`
  - Text from: `status-update` (messages) and `artifact-update` (artifacts)
  - Tool calls: extracted from `data` parts with `functionCall`/`functionResponse`
- **Sessions**: Uses `contextId` from JSON-RPC responses
- **PipelineFinalAgent**: Distinguishes "thinking" text from final output in internal pipelines

### 1.3 ADK (Google Agent Development Kit) — `proxy/adk.go`

- **Request**: Creates a session (`POST /apps/{appName}/users/{userId}/sessions`) + streaming (`POST /run_sse`)
- **Response**: The most complex proxy (~715 lines):
  - **Context rotation**: Detects `<context-limit-summary>`, creates a new session, and retries
  - **Deduplication**: Filters `partial=true` vs `partial=false` text
  - **Keep-alive**: Sends SSE comments every 10s
  - **Thoughts**: Parts marked as `Thought` are emitted as "thinking"
- **Sessions**: Explicit management via the ADK agent's API

### Routing

`proxy/proxy.go:77-93` — a simple switch on `agent.Protocol`:

```go
switch agent.Protocol {
case "custom": return p.rest.Handle(...)
case "a2a":    return p.a2a.Handle(...)
case "adk":    return p.adk.Handle(...)
}
```

---

## 2. Custom Agents (internal agents)

A different concept from external agents: they run an **LLM + tool-calling loop inside the API itself**.

### Model (`models/custom_agent.go`)

| Field | Description |
|-------|-------------|
| `SystemPrompt` | Prompt that defines the behavior |
| `LLMModelID` | Reference to an LLM model in the DB (Anthropic, OpenAI, Google) |
| `SubAgentIDs` | Regular agents invocable as tools |
| `MCPServerIDs` | MCP servers whose tools are available |
| `Visibility` | `private` / `shared` / `public` with full RBAC |

### Runtime (`customagent/runtime.go`)

Uses the shared `mcp.RunLoop` loop with parallel tool execution:

```
1. Resolves the LLM model (fallback to default)
2. Builds tools (MCP servers + sub-agents) with permission verification
3. Delegates to mcp.RunLoop with Parallel=true:
   ├── Calls the LLM (max 10 rounds)
   ├── If there are tool calls → executes in parallel:
   │   ├── MCP: callMCPTool → server.Client.CallTool
   │   └── Agent: callSubAgent → REST/A2A/ADK client
   ├── Feeds results back to the LLM
   └── If there are no tool calls → emits final text
```

### Tools (`customagent/tools.go`)

- MCP tools: prefix `mcp__{serverID}__{toolName}`
- Sub-agent tools: prefix `agent__{agentID}` with schema `{"message": "..."}`
- Verifies the user's permissions on each MCP server and sub-agent

---

## 3. Multi-Agent (2 modes)

### 3.1 Broadcast (`handlers/broadcast.go`)

Sends the same message to **multiple agents in parallel**:

```
1. For each agent (goroutine):
   ├── PrepareMessagesForMultiAgent (computes delta)
   ├── Proxy.Handle → proxy by protocol
   ├── Captures SSE via pipeResponseWriter
   └── Enriches events with agentId, sends to eventCh
2. Multiplexes eventCh → ResponseWriter (unified SSE)
3. Saves results per agent (AddMessage with atomic Lua)
```

### 3.2 Conversation (`handlers/conversation.go`)

**Sequential** execution through a defined sequence:

```
Sequence: ["agentA", "agentB", "user", "agentC"]
                                  ↑
                         Stops and waits for input

1. Loop over Sequence from SequenceIndex:
   ├── If step is "user" → emits step event, updates index, returns
   └── If step is an agent:
       ├── PrepareMessagesForConversation (delta + round responses)
       ├── Proxy to the agent via pipeResponseWriter
       ├── Enriches events, streams inline
       ├── Saves message, accumulates roundResponses
       └── Updates SequenceIndex (atomic Lua)
2. At the end: reset index to 0 (cyclic)
```

### Context Delta (`proxy/context.go`)

- `PrepareMessagesForMultiAgent`: Messages from other agents since the target agent's last interaction
- `PrepareMessagesForConversation`: Delta + accumulated responses from the current round
- Optionally summarizes context via the LLM (`summarizer` package)

---

## 4. Shared Loop (`mcp/loop.go`)

The LLM + tool-calling loop is used by MCP chat and custom agents:

```go
mcp.RunLoop(ctx, w, mcp.LoopParams{
    SessionID:    sessionID,
    SessionStore: sessionStore,
    LLMClient:    llmClient,
    Tools:        tools,
    Messages:     llmMessages,
    Handler:      toolHandler,   // Resolve + Execute
    Parallel:     true/false,    // Parallel (custom agents) or sequential (MCP)
})
```

The `ToolHandler` abstracts the resolution (name + serverID) and execution of each tool call:

| Caller | Resolve | Execute | Parallel |
|--------|---------|---------|----------|
| MCP Chat (single) | `tc.Name, "", false` | `server.Client.CallToolWithHeaders` | No |
| MCP Chat (multi) | Parse `serverID__toolName` | Route to correct server | No |
| Custom Agent | MCP: `realName, serverID` / Agent: `tc.Name` | MCP tool or sub-agent | Yes |

---

## 5. Sessions

### Storage (Redis, TTL 7 days)

| Redis Key | Purpose |
|-------------|-----------|
| `session:{uuid}` | Session data (JSON) |
| `user_sessions:{email}:{agentId}` | Set of IDs per user/agent |
| `user_multi_sessions:{email}` | Set of multi-agent sessions |
| `agent_session_map:{sessionID}:{agentID}` | UI session → agent session mapping |

### Session types

| Type | Creation | Specifics |
|------|----------|-----------------|
| Single-agent | First interaction | Simple key `session:{uuid}` |
| Multi-agent | `POST /api/sessions/multi` | Contains `AgentIDs`, `MultiAgentMode`, `Sequence` |
| Custom agent | First interaction | Same store, prefix `custom:{agentID}` |

### Atomic operations (Lua scripts)

- `AddMessage`: Atomic append of messages (critical in broadcast with concurrent writes)
- `UpdateSequenceIndex`: Atomic update of the sequence index

### Session mapping per protocol

Each external agent maintains its own session. The API maps the internal `sessionID` → the agent's `sessionID`:

- **REST**: `session_id` or `conversation_id` in SSE events
- **A2A**: `contextId` in JSON-RPC responses
- **ADK**: Explicit creation via the API, with rotation on context-limit

---

## 6. Abstractions

| Layer | File | Rationale |
|------|---------|---------------|
| `AgentWrapper` (mutex) | `agents/agent.go` | Concurrent health checks vs reads |
| `Registry` (cache + refresh) | `agents/registry.go` | Avoids DB queries on every request |
| `Proxy` (router) | `proxy/proxy.go` | 30 lines, separates 3 proxies of 100-700 lines |
| `SSEWriter` | `proxy/sse.go` | Used by all 3 proxies, thread-safe |
| `pipeResponseWriter` | `handlers/broadcast.go` | Multiplexes broadcast/conversation |
| `RunLoop` | `mcp/loop.go` | Shared loop between MCP chat and custom agents |
| `ToolHandler` | `mcp/loop.go` | Abstracts tool resolution and execution |
