# Show HN (paste-ready)

Submit as a **Show HN** with the GitHub repo as the URL. Do not use `https://agentgram.eu/mcp` — that host is the docs site.

## Submission

| Field | Value |
| ----- | ----- |
| **Title** | `Show HN: Agentgram – one MCP gateway for all your MCP servers and agents` |
| **URL** | `https://github.com/dfradehubs/agentgram` |

Title is 72 characters (HN limit is 80).

## First comment (paste as-is)

Every team I worked with ended up with a pile of MCP servers and agents (logs, k8s, metrics, SaaS), and everyone wiring each one into each client with its own login and API key. Agentgram is a self-hosted Go gateway: one MCP endpoint in front of all of them, per-user RBAC, the right credential for each upstream (your token, a group API key or a per-user OAuth grant), and an audit log. Agents speak REST/SSE, A2A or Google ADK and show up as MCP tools too. There's a chat console on top, where groups are moderated debates — an LLM picks who speaks.

```
curl -fsSL https://raw.githubusercontent.com/dfradehubs/agentgram/main/docker-compose.yaml -o docker-compose.yaml
docker compose up -d
# http://localhost:3000 — a demo agent is already registered
```

```
claude mcp add --transport http agentgram http://localhost:8080/mcp
```

MIT. Not a ChatGPT clone (that's Open WebUI) and not only an MCP gateway (that's MCPJungle / agentgateway). Docs: https://agentgram.eu

## Timing

Post on a weekday morning US time. Do not post until a release that includes the compose quickstart and demo agent is on `main` / GHCR, or the curl command 404s the new files.
