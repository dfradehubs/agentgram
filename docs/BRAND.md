# Brand collision

There is another product named **AgentGram** at [github.com/agentgram/agentgram](https://github.com/agentgram/agentgram) (`agentgram.co`, org `@agentgram`). It is an agent social network / MCP governance tool. It is **not** this repository.

This project is **dfradehubs/agentgram** and **https://agentgram.eu**.

## Impact

- Google/GitHub search for "agentgram" surfaces their org, site, and Twitter first.
- Snippets that used `https://agentgram.eu/mcp` were a 404 (docs-only host). Localhost is the honest URL until a real hosted demo exists.

## Options (do not rush a rename)

1. **Keep Agentgram.eu** and always qualify: "Agentgram (dfradehubs) — agent + MCP front door". Add GitHub topics. Distinct logo/tagline.
2. **Move the GitHub repo** into an organization (`agentgram-eu` / `agentgram-hq`) so it does not look like a personal dump.
3. **Rename** only if search stays unwinnable after a public launch. A rename is expensive (MCP client configs, images, Helm). Candidate names should be greppable and unclaimed (`frontgate`, `agentdoor`, `muxagent` — check first).

Recommendation: ship the getting-started funnel and a launch post **before** renaming. Revisit after two weeks of traffic data.
