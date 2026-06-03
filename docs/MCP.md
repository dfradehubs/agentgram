# Agentgram MCP - Integrate agents into Claude Code

Agentgram exposes an MCP (Model Context Protocol) endpoint that lets you use the deployed agents as tools from Claude Code, Cursor, or any compatible MCP client.

The server implements the full OAuth flow in a standard way: **Protected Resource Metadata (RFC 9728)**, **Authorization Server Metadata (RFC 8414)**, and **Dynamic Client Registration (RFC 7591)**. This means most clients connect **with just the URL**, without configuring `clientId`, scopes, or endpoints: the client discovers the authorization server and registers the client automatically.

## Requirements

- [Claude Code](https://claude.com/claude-code) (CLI) or any compatible MCP client
- A Google Workspace account with access to Agentgram

## Setup

### Claude Code

```bash
# Add the MCP server (OAuth + DCR are resolved automatically)
claude mcp add --transport http agentgram https://agentgram.example.com/mcp
```

On first use, Claude Code discovers the authorization server, registers the client via DCR, and opens the browser for login. There is no need to pass `clientId` or `authServerMetadataUrl`.

**Recommended**: increase the MCP output token limit for long agent responses. Add this to `~/.claude/settings.json`:

```json
{
  "env": {
    "MAX_MCP_OUTPUT_TOKENS": "50000"
  }
}
```

This allows responses of up to 50K tokens (the default is 25K). Agents like logs-agent or sentry-agent can return detailed responses with tables and breakdowns that need more space.

**Recommended**: allow the agentgram tools automatically to prevent the confirmation prompt from blocking the parallel execution of multiple agents:

```json
{
  "permissions": {
    "allow": [
      "mcp__agentgram__*"
    ]
  }
}
```

Without this, Claude Code asks for confirmation on every tool call, which prevents running several agents in parallel (the first agent's prompt blocks the second).

### Cursor

In `~/.cursor/mcp.json` (global) or `.cursor/mcp.json` (project):

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

> Cursor runs Dynamic Client Registration (RFC 7591) against the `registration_endpoint` advertised by the metadata. Agentgram advertises itself as the authorization server (in `oauth-protected-resource`), so the DCR goes to `https://agentgram.example.com/register`, which statically returns the public client `agentgram-mcp` — no need for real DCR in Keycloak or any manual configuration.

### Cursor automations

Cursor's **automations** (background agents) reuse the same MCP configuration (`~/.cursor/mcp.json` or `.cursor/mcp.json`). Once the `agentgram` block above is added, it is also available to the automations; each one will use the already-authorized OAuth token.

### Generic (any MCP client)

The Agentgram MCP server uses HTTP Streamable transport with OAuth2 PKCE:

| Parameter | Value |
|---|---|
| **URL** | `https://agentgram.example.com/mcp` |
| **Transport** | HTTP (Streamable) |
| **OAuth2 Authorization Server (effective)** | `https://keycloak.example.com/realms/mcp-servers` (the `authorize`/`token` endpoints point there) |
| **Client ID** | `agentgram-mcp` (public, PKCE, no secret) — registered automatically via DCR |
| **Callback** | `http://localhost:<port>/callback` (dynamic port; the Keycloak client allows a wildcard in `redirectUris`) |
| **Scopes** | `openid profile email groups offline_access` |
| **Protected Resource Metadata** | `https://agentgram.example.com/.well-known/oauth-protected-resource` |
| **Authorization Server Metadata** | `https://agentgram.example.com/.well-known/oauth-authorization-server` |
| **Dynamic Client Registration** | `https://agentgram.example.com/register` (RFC 7591; always returns the `agentgram-mcp` client) |

> **The client only needs the `/mcp` URL.** The `oauth-protected-resource` advertises Agentgram as its own authorization server, so discovery (RFC 8414) and DCR (RFC 7591) happen against `agentgram.example.com`. Agentgram acts as a facade: it proxies the real Keycloak endpoints (`realms/mcp-servers`) but overrides the `registration_endpoint` to its `/register` and filters `scopes_supported` down to the ones `agentgram-mcp` accepts. This prevents the client from going directly to Keycloak's `clients-registrations` (blocked by Istio) or requesting unregistered scopes (`invalid_scope`).

## Authentication

The first time you use the MCP, your client will redirect you to Keycloak to authenticate with your Google Workspace account. The token is stored locally and refreshed automatically.

In Claude Code:

```
/mcp
# Select agentgram -> Authenticate
# The browser opens -> Log in with Google -> Done
```

To re-authenticate or clear credentials:

```
/mcp
# Select agentgram -> Clear authentication
```

## Available tools

Tools are generated dynamically based on the agents you have access to. Each agent is exposed as an `ask_<agent-id>` tool:

| Tool | Agent | Description |
|---|---|---|
| `ask_anton` | Anton | Knowledge management, search across postmortems and internal documentation |
| `ask_antonia` | Antonia | Database infrastructure (Cloud SQL, secrets, metadata) |
| `ask_kube-agent` | Kubernetes Agent | Management of K8s clusters, pods, deployments, logs |
| `ask_metrics-agent` | Metrics Agent | Querying Prometheus/Grafana metrics |
| `ask_logs-agent` | Logs Agent | Log analysis in OpenSearch |
| `ask_traces-agent` | Traces Agent | Distributed traces with Grafana Tempo |
| `ask_coding-agent` | Coding Agent | Code and PR operations on GitHub |
| `ask_sentry-agent` | Sentry Agent | Errors, stack traces, and performance in Sentry |
| `list_agents` | (utility) | Lists all available agents |

Each tool accepts:
- `question` (required): The question or task for the agent
- `session_id` (optional): Session ID to continue a previous conversation

## Usage examples

### Investigate an incident

```
Ask logs-agent if there are errors in the last 30 minutes of the payment-api service.
Then ask traces-agent for traces with 5xx errors in the same service.
Finally, ask metrics-agent for the CPU and memory usage of payment-api in the last hour.
```

### Kubernetes operations

```
Ask kube-agent how many pods there are in the production namespace and whether any are in CrashLoopBackOff.
```

### Query internal knowledge

```
Ask Anton about the rollback procedure for the main database.
```

### Analyze errors in Sentry

```
Ask sentry-agent for the most frequent errors in the last hour in the example-web project.
```

### Query database infrastructure

```
Ask Antonia about the status of the Cloud SQL instances and whether there are any active alerts.
```

### Combining agents

```
I need to investigate why the search-api service is slow:
1. Ask metrics-agent for the p99 latency metrics of search-api in the last hour
2. Ask traces-agent for the slowest traces of search-api
3. Ask logs-agent if there are any related errors or warnings
```

## Architecture

```
Claude Code / Cursor / MCP Client
        |
        | POST /mcp (JSON-RPC 2.0 + Bearer JWT)
        v
   Agentgram API (/mcp endpoint)
        |
        | 1. Validates JWT (Keycloak, signature + issuer)
        | 2. Resolves tool -> agent
        | 3. Verifies the user's permissions
        | 4. Proxies to the agent (A2A / ADK / Custom)
        | 5. Returns the response as an MCP tool result
        v
   Remote agent (K8s)
```

## Troubleshooting

### "needs authentication"

Run `/mcp` in Claude Code, select agentgram, and re-authenticate.

### "Agent call failed"

Verify that the agent is healthy:

```bash
curl -s https://agentgram.example.com/api/agents | jq '.[] | {id, status}'
```

### Tools do not appear

Restart Claude Code or run `/mcp` to reconnect. Tools are discovered during the initial handshake.

### Permission error

The visible agents depend on your Google Workspace groups. Contact the SRE team if you need access to a specific agent.
