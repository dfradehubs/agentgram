---
title: Documentation
next: getting-started
---

Welcome to the Agentgram documentation.

Agentgram is a unified interface for interacting with multiple AI agents. The **API** is a Go
multiplexer that connects to remote agents over their native protocol (Custom REST/SSE, A2A or
Google ADK), handles authentication, permissions and session storage, and converts everything to a
uniform stream of [AG-UI](https://docs.ag-ui.com/) events. The **web** client renders that stream.

## Get started

{{< cards >}}
  {{< card link="/docs/getting-started/" title="Getting Started" icon="play" subtitle="Run the full stack locally in a couple of commands." >}}
  {{< card link="/docs/docker-compose/" title="Docker Compose" icon="server" subtitle="Self-host API + web + Redis + PostgreSQL on a server." >}}
  {{< card link="/docs/kubernetes/" title="Kubernetes" icon="cube" subtitle="Deploy with Helm using the bjw-s app-template chart." >}}
{{< /cards >}}

## Learn the concepts

{{< cards >}}
  {{< card link="/docs/configuration/" title="Configuration" icon="adjustments" subtitle="The config file, environment variables and secrets." >}}
  {{< card link="/docs/agents/" title="Agents & protocols" icon="switch-horizontal" subtitle="Register agents and how each protocol maps to AG-UI." >}}
  {{< card link="/docs/mcp/" title="MCP" icon="terminal" subtitle="Expose your agents as tools in any MCP-compatible IDE or CLI." >}}
  {{< card link="/api/" title="API Reference" icon="code" subtitle="The full OpenAPI reference for the HTTP API." >}}
{{< /cards >}}
