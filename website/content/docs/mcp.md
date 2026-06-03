---
title: MCP for Claude Code
weight: 6
---

Agentgram exposes an MCP endpoint that turns your agents into tools inside **Claude Code** and
**Cursor**. It implements the full discovery and auth flow — protected resource metadata
(RFC 9728), authorization server discovery (RFC 8414) and **Dynamic Client Registration**
(RFC 7591) — so there is nothing to configure by hand.

## Add the server

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

## Authenticate

In Claude Code, run `/mcp`, select `agentgram`, and sign in once. The client discovers and registers
everything automatically — no client ID, scopes or endpoints to set by hand.

## Use it

Tools are generated dynamically from the agents you have access to. Each agent becomes an
`ask_<agent-id>` tool:

```text
Ask logs-agent for the errors in the last 30 minutes of the payment-api service.
Then ask metrics-agent for its CPU and memory usage in the last hour.
```

## Tips

- Bump `MAX_MCP_OUTPUT_TOKENS` (e.g. to `50000`) in `~/.claude/settings.json` for agents that return
  long, tabular responses.
- Agentgram sends progress notifications every 15s to keep long-running calls alive.

The full guide (Cursor automations, OAuth internals, tuning) is in
[`docs/MCP.md`](https://github.com/dfradehubs/agentgram/blob/main/docs/MCP.md).
