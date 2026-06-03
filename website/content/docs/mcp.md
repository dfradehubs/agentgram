---
title: Model Context Protocol (MCP)
weight: 6
---

Agentgram ships a standard **MCP server** that turns your agents into tools for any
MCP-compatible IDE or CLI. It implements the full discovery and auth flow defined by the spec —
protected resource metadata (RFC 9728), authorization server discovery (RFC 8414) and
**Dynamic Client Registration** (RFC 7591) — so any conformant client connects with **just the URL**.
No client ID, scopes or endpoints to configure by hand.

That includes editors and agents such as **Claude Code**, **Cursor**, and any other tool that speaks
the MCP HTTP transport with OAuth.

## Add the server

The only thing a client needs is the `/mcp` URL.

**Claude Code:**

```bash
claude mcp add --transport http agentgram https://agentgram.example.com/mcp
```

**Cursor** (or any client using an `mcp.json`) — in `~/.cursor/mcp.json` (global) or
`.cursor/mcp.json` (project):

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

Other MCP clients follow the same pattern: point them at the `/mcp` endpoint and let the standard
discovery + DCR flow do the rest.

## Authenticate

On first use the client runs the OAuth flow automatically (it discovers the authorization server and
registers itself via DCR). In Claude Code, run `/mcp`, select `agentgram`, and sign in once.

## Use it

The exposed toolset is generated dynamically from what **you** are allowed to use:

- **One `ask_<agent-id>` tool per agent** you have access to.
- **The tools of every MCP server registered in Agentgram** (from the admin panel) that you have
  permission to use. Agentgram aggregates those upstream MCP servers and re-exposes their tools
  through its own endpoint, so a single connection gives your client both the agents and the tools.

Everything is filtered by your identity and permissions — you only ever see what you're entitled to.

```text
Ask logs-agent for the errors in the last 30 minutes of the payment-api service.
Then ask metrics-agent for its CPU and memory usage in the last hour.
```

## Tips

- Bump `MAX_MCP_OUTPUT_TOKENS` (e.g. to `50000`) in your client settings for agents that return
  long, tabular responses.
- Agentgram sends progress notifications every 15s to keep long-running calls alive.

The full guide (client specifics, OAuth internals, tuning) is in
[`docs/MCP.md`](https://github.com/dfradehubs/agentgram/blob/main/docs/MCP.md).
