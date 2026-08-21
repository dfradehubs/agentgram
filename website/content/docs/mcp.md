---
title: Model Context Protocol (MCP)
weight: 7
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
claude mcp add --transport http agentgram http://localhost:8080/mcp
```

**Cursor** (or any client using an `mcp.json`) — in `~/.cursor/mcp.json` (global) or
`.cursor/mcp.json` (project):

```json
{
  "mcpServers": {
    "agentgram": {
      "type": "http",
      "url": "http://localhost:8080/mcp"
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
- **One `group__<group-id>` tool per [agent group]({{< relref "collaboration" >}}#moderated-group-debates)**
  you can participate in: the question triggers a **moderated debate** where an LLM moderator picks
  the most relevant agents to answer in sequence, and you get their combined replies (plus an
  optional synthesis when several agents contribute) as one result.
- **The tools of every MCP server registered in Agentgram** (from the admin panel) that you have
  permission to use. Agentgram aggregates those upstream MCP servers and re-exposes their tools
  through its own endpoint, so a single connection gives your client both the agents and the tools.
- **One `skill__<skill-id>` tool per skill** you can read: calling it returns the skill's stored
  instructions verbatim.
- **The `admin_*` tools**, if your role is `editor` or `admin` (see below).

Everything is filtered by your identity and permissions — you only ever see what you're entitled to.

```text
Ask logs-agent for the errors in the last 30 minutes of the payment-api service.
Then ask metrics-agent for its CPU and memory usage in the last hour.
```

## Administer from your client

Editors and admins also get the administration surface as tools, so managing the instance doesn't
require leaving your editor. Ordinary users see none of these.

| Tools | Role | What they do |
|---|---|---|
| `admin_list_agents`, `admin_get_agent`, `admin_create_agent`, `admin_update_agent` | editor | Full agent configuration: read, register, reconfigure |
| `admin_list_mcp_servers`, `admin_get_mcp_server`, `admin_create_mcp_server`, `admin_update_mcp_server` | editor | Same, for upstream MCP servers |
| `admin_delete_agent`, `admin_delete_mcp_server` | admin | Delete a resource |
| `admin_query_audit` | admin | Query the audit log |
| `admin_metrics` | admin | Query observability metrics (per instance, user, agent or MCP server) |
| `admin_get_settings`, `admin_update_settings` | admin | Read and change the runtime settings |

These tools replay their request against the same `/api/admin` endpoints the admin panel uses, so
validation, permissions and side effects behave identically. Three things make them safe to expose:

- **Credentials never leave the server.** Bearer tokens, API keys, OAuth2 secrets and header values
  come back as `"***"`. Sending a redacted value back in an update keeps the stored credential, so a
  read-edit-write round-trip can't wipe it.
- **Write tools ask for confirmation.** They carry the MCP `destructiveHint` annotation and an
  explicit warning, so a conformant client checks with you before running one.
- **Everything is audited.** Every configuration change — from the admin panel *or* from these
  tools — lands in the audit log with your identity, the payload, the outcome and which surface it
  came from. Denied attempts included.

## Tips

- Bump `MAX_MCP_OUTPUT_TOKENS` (e.g. to `50000`) in your client settings for agents that return
  long, tabular responses.
- Agentgram sends progress notifications every 15s to keep long-running calls alive.

The full guide (client specifics, OAuth internals, tuning) is in
[`docs/MCP.md`](https://github.com/dfradehubs/agentgram/blob/main/docs/MCP.md).
