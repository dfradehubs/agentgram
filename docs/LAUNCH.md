# Launch notes

These are drafts. Do **not** paste a live MCP URL on agentgram.eu — that host is the docs site and `/mcp` is 404.

## Positioning (one sentence)

Agentgram is the self-hosted front door for every agent and MCP server you already run: one chat, one MCP endpoint, RBAC on both.

## Show HN

**Title:** Show HN: Agentgram – one MCP endpoint and one chat for every agent you already run

**Body:**

I kept ending up with a different chat window for every agent (logs, k8s, metrics), each on a different protocol. Agentgram is a Go multiplexer that speaks REST/SSE, A2A and Google ADK, converts everything to AG-UI, and exposes the same agents as MCP tools for Cursor / Claude Code.

```
curl -fsSL https://raw.githubusercontent.com/dfradehubs/agentgram/main/docker-compose.yaml -o docker-compose.yaml
docker compose up -d
# http://localhost:3000  — demo agent is already there
```

MIT, self-hosted. Not a ChatGPT clone (use Open WebUI for that) and not only an MCP gateway (use MCPJungle / agentgateway if you do not want a chat UI).

Repo: https://github.com/dfradehubs/agentgram
Docs: https://agentgram.eu

## r/selfhosted

Title: Agentgram – self-hosted front door for AI agents and MCP (Go + Next.js, MIT)

Same body as Show HN, plus: Redis + Postgres in the compose file; laptop mode embeds both so you can run the API as one process.

## r/LocalLLaMA / MCP / AG-UI discords

Lead with the MCP one-liner and the compose curl. Link the compare page so people who wanted Open WebUI self-select out.

## After posting

- Do not add features for a week; watch issues and discussions.
- Measure: GitHub views, clones, time until first issue from a stranger.
- If people bounce: the demo agent or the compose file is the bug, not the protocol matrix.
