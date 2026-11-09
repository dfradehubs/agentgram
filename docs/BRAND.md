# Brand collision

There is another product named **AgentGram** at [github.com/agentgram/agentgram](https://github.com/agentgram/agentgram) (`agentgram.co`, org `@agentgram`). It is an agent social network / MCP governance tool. It is **not** this repository.

This project is **dfradehubs/agentgram** and **https://agentgram.eu**.

## Impact

- Google/GitHub search for "agentgram" surfaces their org, site, and Twitter first.
- Snippets that used `https://agentgram.eu/mcp` were a 404 (docs-only host). Localhost is the honest URL until a real hosted demo exists.

## Options (do not rush a rename)

1. **Keep Agentgram.eu** and always qualify: "Agentgram (agentgram.eu) — MCP & agent gateway". Add GitHub topics. Distinct logo/tagline.
2. **Move the GitHub repo** into an organization (`agentgram-eu` / `agentgram-hq`) so it does not look like a personal dump.
3. **Rename** only if search stays unwinnable after a public launch. A rename is expensive (MCP client configs, images, Helm). Candidate names should be greppable and unclaimed (`frontgate`, `agentdoor`, `muxagent` — check first).

Recommendation: ship the getting-started funnel and a launch post **before** renaming. Revisit after two weeks of traffic data.

## Decision (2026-11): reposition, keep the name

Agentgram is positioned as a **self-hosted gateway for MCP servers and agents** — one MCP endpoint and one API, per-user RBAC and credentials, audit — with the chat and admin console as a secondary, included surface. The name and domain stay.

Because agentgram.co also describes itself as MCP governance, always pair the name with the domain or the category ("Agentgram (agentgram.eu) — MCP & agent gateway") in titles, launch posts and listings, and keep the GitHub topics `mcp-gateway`, `mcp-proxy` and `agent-gateway`.
