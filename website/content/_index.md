---
title: "Agentgram — One chat, every agent"
layout: hextra-home
---

{{< hextra/hero-badge >}}
  <div class="hx:w-2 hx:h-2 hx:rounded-full hx:bg-primary-400"></div>
  <span>Open source · MIT</span>
  {{< icon name="arrow-circle-right" attributes="height=14" >}}
{{< /hextra/hero-badge >}}

<div class="hx:mt-6 hx:mb-6">
{{< hextra/hero-headline >}}
  One chat. Every agent.&nbsp;<br class="hx:sm:block hx:hidden" />Any protocol.
{{< /hextra/hero-headline >}}
</div>

<div class="hx:mb-12">
{{< hextra/hero-subtitle >}}
  Centralize every agent and MCP server in your company — ADK, A2A or custom —&nbsp;<br class="hx:sm:block hx:hidden" />behind one API and one MCP endpoint, with RBAC deciding who can reach what.
{{< /hextra/hero-subtitle >}}
</div>

<div class="hx:mb-6">
{{< hextra/hero-button text="Get Started" link="/docs/" >}}
&nbsp;
{{< hextra/hero-button text="View on GitHub" link="https://github.com/dfradehubs/agentgram" style="background:#27272a;border:1px solid #3f3f46;" >}}
</div>

<div class="hx:mt-6"></div>

{{< hextra/feature-grid >}}
  {{< hextra/feature-card
    title="Protocol-agnostic"
    icon="switch-horizontal"
    subtitle="Custom REST/SSE, A2A (JSON-RPC) and Google ADK agents — all behind one uniform interface."
  >}}
  {{< hextra/feature-card
    title="AG-UI native"
    icon="rss"
    subtitle="The API emits standard AG-UI events over SSE, so the UI never has to know how an agent is built."
  >}}
  {{< hextra/feature-card
    title="Sessions that persist"
    icon="collection"
    subtitle="Conversation history per agent, stored in Redis and managed by the API. Agents stay stateless."
  >}}
  {{< hextra/feature-card
    title="Multi-agent chats"
    icon="user-group"
    subtitle="Talk to several agents in one thread and propagate context between them."
  >}}
  {{< hextra/feature-card
    title="RBAC for every agent & MCP"
    icon="shield-check"
    subtitle="Fine-grained, per-agent and per-MCP access control by group or user. One endpoint; each caller only sees what they're allowed to."
  >}}
  {{< hextra/feature-card
    title="MCP server"
    icon="terminal"
    subtitle="Aggregate your agents and company MCP servers into one MCP endpoint for any compatible IDE or CLI — standard OAuth + Dynamic Client Registration, no manual setup."
  >}}
  {{< hextra/feature-card
    title="Built-in observability"
    icon="chart-bar"
    subtitle="Usage, latency and cost dashboards out of the box. Plus a Slack integration."
  >}}
  {{< hextra/feature-card
    title="Deploy anywhere"
    icon="cube"
    subtitle="Published container images, a Docker Compose stack and Helm values for Kubernetes."
  >}}
{{< /hextra/feature-grid >}}
