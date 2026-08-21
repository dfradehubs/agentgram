# Show HN (paste-ready)

Submit as a **Show HN** with the GitHub repo as the URL. Do not use `https://agentgram.eu/mcp` — that host is the docs site.

## Submission

| Field | Value |
| ----- | ----- |
| **Title** | `Show HN: Agentgram – one MCP endpoint and a chat for all your agents` |
| **URL** | `https://github.com/dfradehubs/agentgram` |

Title is 68 characters (HN limit is 80).

## First comment (paste as-is)

I kept ending up with a different chat window for every agent (logs, k8s, metrics), each on a different protocol. Agentgram is a self-hosted Go multiplexer: REST/SSE, A2A and Google ADK in, AG-UI out, same agents as MCP tools for Cursor / Claude Code, with RBAC on both.

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
