---
title: Compare
weight: 8
---

Agentgram is **not** another ChatGPT clone. It is a **front door**: one chat UI and one MCP
endpoint in front of the agents and tools you already run, with RBAC.

| | Agentgram | Open WebUI | MCPJungle | IBM ContextForge | agentgateway |
| --- | --- | --- | --- | --- | --- |
| Job to be done | Company/team front door for heterogeneous agents + MCP | Chat with Ollama / OpenAI-compatible models | One MCP endpoint for many MCP servers | MCP/A2A/REST federation + governance | High-performance MCP/A2A/LLM data plane |
| Chat UI | Yes (AG-UI streaming) | Yes (primary) | Admin UI | Admin UI | Built-in explorer |
| Bring your own agent protocols | REST/SSE, A2A, Google ADK | Model APIs, tools | MCP servers | MCP, A2A, REST/gRPC | MCP, A2A, LLM |
| MCP gateway + OAuth/DCR | Yes | Via MCPO / plugins | Yes | Yes | Yes |
| Per-agent / per-MCP RBAC | Yes | Roles in the chat product | Access control | Governance / plugins | Policy on the data plane |
| Typical install | `curl` compose file, or one image + Redis/Postgres | `docker run` | `brew` / compose | Python package / compose | Single binary |
| Licence | MIT | Custom (branding limits) | MPL | Apache-2.0 | Apache-2.0 |

**Pick Agentgram** if you have (or are about to have) several agents that do not share a protocol,
and you want one place for humans (chat) and IDEs (MCP) with the same permissions.

**Pick Open WebUI** if you want a polished chat UI in front of Ollama or an OpenAI-compatible API.

**Pick MCPJungle / ContextForge / agentgateway** if you only need a gateway and already have a chat
client (Claude, Cursor, Copilot).
