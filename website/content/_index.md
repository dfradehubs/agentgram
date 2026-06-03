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
{{< hextra/hero-button text="Get Started" link="/docs/getting-started/" >}}
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
    title="Share & collaborate"
    icon="share"
    subtitle="Share conversations with revocable links, and build shared multi-agent groups your team uses together."
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

<section class="ag-section">
<div class="ag-section-head">
<span class="ag-eyebrow">How it works</span>
<h2 class="ag-section-title">Three steps to one front door for every agent</h2>
<p class="ag-section-sub">Register what you already run, put it behind a single gateway, and reach it from anywhere — without each client learning a new protocol.</p>
</div>

{{% steps %}}

### Register any agent

Point Agentgram at an agent's endpoint and pick its protocol — **Custom REST/SSE**, **A2A (JSON-RPC)** or **Google ADK**. No redeploy: agents are managed at runtime from the admin panel and stored in PostgreSQL.

### One API, one MCP endpoint — behind RBAC

Every agent and every company MCP server is reachable through a single API and a single `/mcp` endpoint. Role-based access control decides, per agent and per MCP server, exactly who can reach what.

### Talk to it from anywhere

Chat from the web UI, call agents as tools from your IDE or CLI over MCP, or wire them into Slack. The API converts every protocol into a uniform stream of AG-UI events.

{{% /steps %}}
</section>

<section class="ag-section">
<div class="ag-section-head">
<span class="ag-eyebrow">Architecture</span>
<h2 class="ag-section-title">A thin, stateless multiplexer in the middle</h2>
<p class="ag-section-sub">Clients speak AG-UI and MCP. Agents speak their own protocol. The Go API translates between them, enforces access, and keeps the sessions.</p>
</div>

<div class="ag-arch">
  <div class="ag-arch-col">
    <span class="ag-arch-label">Clients</span>
    <div class="ag-node">Web UI</div>
    <div class="ag-node">IDE / CLI · MCP</div>
    <div class="ag-node">Slack</div>
  </div>
  <div class="ag-arch-flow">AG-UI · MCP →</div>
  <div class="ag-arch-col ag-arch-core">
    <span class="ag-arch-label">Agentgram API · Go</span>
    <div class="ag-node ag-node-core">JWT auth · OIDC</div>
    <div class="ag-node ag-node-core">RBAC per agent &amp; MCP</div>
    <div class="ag-node ag-node-core">Protocol mux → AG-UI</div>
    <div class="ag-node ag-node-core">Session store</div>
  </div>
  <div class="ag-arch-flow">→ native protocol</div>
  <div class="ag-arch-col">
    <span class="ag-arch-label">Agents &amp; tools</span>
    <div class="ag-node">Custom REST / SSE</div>
    <div class="ag-node">A2A · JSON-RPC</div>
    <div class="ag-node">Google ADK</div>
    <div class="ag-node">MCP servers</div>
  </div>
</div>
<div class="ag-arch-stores">
  <div class="ag-store">Redis · sessions &amp; pub/sub</div>
  <div class="ag-store">PostgreSQL · groups &amp; persistent data</div>
</div>
</section>

<section class="ag-section">
<div class="ag-section-head">
<span class="ag-eyebrow">Bring any agent</span>
<h2 class="ag-section-title">Different protocols in. One event stream out.</h2>
<p class="ag-section-sub">Whatever an agent speaks, the web client always sees the same AG-UI events — so the UI never changes when your agents do.</p>
</div>

{{< tabs >}}
  {{< tab name="Custom REST/SSE" >}}
A plain HTTP endpoint that streams Server-Sent Events (or returns JSON). The simplest way to plug in your own service.

```text
POST /api/agents/{agentId}/chat
Content-Type: application/json

{ "messages": [{ "role": "user", "content": "Hello" }],
  "session_id": "optional-uuid" }
```
  {{< /tab >}}
  {{< tab name="A2A (JSON-RPC)" >}}
Agent-to-Agent over JSON-RPC. Register the endpoint and Agentgram drives the A2A handshake for you.

```json
{ "jsonrpc": "2.0", "method": "message/send",
  "params": { "message": { "role": "user",
    "parts": [{ "kind": "text", "text": "Hello" }] } },
  "id": "1" }
```
  {{< /tab >}}
  {{< tab name="Google ADK" >}}
Google's Agent Development Kit over REST/SSE. Set the app name and user id; Agentgram maps it to AG-UI.

```yaml
protocol: adk
endpoint: https://my-adk-agent.internal
adk_app_name: support
adk_user_id: "${user}"
```
  {{< /tab >}}
  {{< tab name="AG-UI out" >}}
Every protocol above is converted to the same AG-UI event stream the web client consumes.

```text
data: {"type":"RUN_STARTED","threadId":"…","runId":"…"}
data: {"type":"TEXT_MESSAGE_START","messageId":"…","role":"assistant"}
data: {"type":"TEXT_MESSAGE_CONTENT","messageId":"…","delta":"Hello"}
data: {"type":"TOOL_CALL_START","toolCallId":"…","toolName":"SearchTool"}
data: {"type":"TEXT_MESSAGE_END","messageId":"…"}
data: {"type":"RUN_FINISHED","threadId":"…","runId":"…"}
```
  {{< /tab >}}
{{< /tabs >}}
</section>

<section class="ag-section">
<div class="ag-section-head">
<span class="ag-eyebrow">What you can build</span>
<h2 class="ag-section-title">One gateway, many workflows</h2>
<p class="ag-section-sub">A single integration that each person experiences differently, based on what they're allowed to reach.</p>
</div>

<div class="ag-cards">
  <div class="ag-card">
    <div class="ag-card-icon">🚨</div>
    <div class="ag-card-title">Incident response</div>
    <p class="ag-card-text">Bundle logs, metrics and Kubernetes agents into a shared group and triage an outage together — everyone sees the same thread.</p>
  </div>
  <div class="ag-card">
    <div class="ag-card-icon">🧰</div>
    <div class="ag-card-title">Agents in your IDE</div>
    <p class="ag-card-text">Add one MCP endpoint to Claude Code or Cursor and call every agent you're allowed to as a tool — <code>ask_logs-agent</code> and more.</p>
  </div>
  <div class="ag-card">
    <div class="ag-card-icon">📊</div>
    <div class="ag-card-title">Cost &amp; latency in the open</div>
    <p class="ag-card-text">Built-in dashboards break down usage, latency and error rate per agent and per user — no extra observability stack.</p>
  </div>
  <div class="ag-card">
    <div class="ag-card-icon">🔗</div>
    <div class="ag-card-title">Share a finding</div>
    <p class="ag-card-text">Send a teammate a read-only, time-limited and revocable link to a conversation. They can clone it to keep digging.</p>
  </div>
  <div class="ag-card">
    <div class="ag-card-icon">🧩</div>
    <div class="ag-card-title">One gateway for the company</div>
    <p class="ag-card-text">Register every agent and MCP server once; RBAC by Google Workspace group or email gives each person only what they're entitled to.</p>
  </div>
  <div class="ag-card">
    <div class="ag-card-icon">🔌</div>
    <div class="ag-card-title">Bring any framework</div>
    <p class="ag-card-text">Custom REST/SSE, A2A or Google ADK — register the endpoint and it speaks AG-UI like everything else.</p>
  </div>
</div>
</section>

<section class="ag-section">
<div class="ag-section-head">
<span class="ag-eyebrow">See it in action</span>
<h2 class="ag-section-title">A real chat client, not a demo widget</h2>
<p class="ag-section-sub">Streaming responses, tool calls, multi-agent threads and an admin surface to manage it all.</p>
</div>

<div class="ag-gallery">
  <figure class="ag-shot">
    <div class="ag-shot-bar"><span></span><span></span><span></span></div>
    <img src="/images/screenshots/chat-single.webp" width="2880" height="1800" loading="lazy" decoding="async" alt="Chatting with a single agent: streaming markdown answer with a collapsed tool call and a results table." />
    <figcaption><strong>Chat with any agent.</strong> Streaming markdown, tool calls and chartable data — whatever protocol the agent speaks.</figcaption>
  </figure>
  <figure class="ag-shot">
    <div class="ag-shot-bar"><span></span><span></span><span></span></div>
    <img src="/images/screenshots/chat-multi-agent.webp" width="2880" height="1800" loading="lazy" decoding="async" alt="A multi-agent group thread where one question is routed to both the Logs and Metrics agents." />
    <figcaption><strong>Multi-agent groups.</strong> Ask one question, route it across several agents, and keep the context in a single shared thread.</figcaption>
  </figure>
  <figure class="ag-shot">
    <div class="ag-shot-bar"><span></span><span></span><span></span></div>
    <img src="/images/screenshots/admin-agents.webp" width="2880" height="1800" loading="lazy" decoding="async" alt="Admin panel listing registered agents with their protocol, status and permissions." />
    <figcaption><strong>Manage agents at runtime.</strong> Register agents, pick their protocol and set per-agent permissions — no redeploy.</figcaption>
  </figure>
  <figure class="ag-shot">
    <div class="ag-shot-bar"><span></span><span></span><span></span></div>
    <img src="/images/screenshots/admin-observability.webp" width="2880" height="1800" loading="lazy" decoding="async" alt="Observability dashboard with request volume, error rate, p95 latency and requests by resource." />
    <figcaption><strong>Observability out of the box.</strong> Requests, error rate, p95 latency and a per-agent breakdown — built in.</figcaption>
  </figure>
</div>
</section>

<section class="ag-section">
<div class="ag-section-head">
<span class="ag-eyebrow">Plug it into your tools</span>
<h2 class="ag-section-title">Your agents as tools, in any MCP client</h2>
<p class="ag-section-sub">Agentgram ships a standard MCP server with OAuth and Dynamic Client Registration — connect with just the URL.</p>
</div>

{{< tabs >}}
  {{< tab name="Claude Code" >}}
```bash
claude mcp add --transport http agentgram https://agentgram.eu/mcp
# then run /mcp, pick "agentgram" and sign in once
```

You get one `ask_<agent-id>` tool per agent you can reach, plus the tools of every MCP server registered in Agentgram — all filtered by your permissions.
  {{< /tab >}}
  {{< tab name="Cursor / mcp.json" >}}
```json
{
  "mcpServers": {
    "agentgram": {
      "type": "http",
      "url": "https://agentgram.eu/mcp"
    }
  }
}
```

On first use the client runs the OAuth flow automatically — discovery + Dynamic Client Registration, no manual setup.
  {{< /tab >}}
{{< /tabs >}}
</section>

<section class="ag-section">
<div class="ag-section-head">
<span class="ag-eyebrow">Deploy anywhere</span>
<h2 class="ag-section-title">From a laptop to a cluster</h2>
<p class="ag-section-sub">Published container images, a Docker Compose stack and Helm values. Database migrations run automatically on startup.</p>
</div>

{{< tabs >}}
  {{< tab name="Docker Compose" >}}
Self-host the whole stack on a single VM.

```bash
cd agentgram/examples/docker-compose
cp .env.example .env      # set POSTGRES_PASSWORD, pin AGENTGRAM_VERSION
docker compose up -d      # web on http://localhost:3000
```
  {{< /tab >}}
  {{< tab name="Kubernetes" >}}
Deploy with the bjw-s `app-template` chart — plain Kubernetes objects, no CRDs.

```bash
helm repo add bjw-s https://bjw-s-labs.github.io/helm-charts
helm install agentgram-api bjw-s/app-template -f values-api.yaml -n agentgram
helm install agentgram-web bjw-s/app-template -f values-web.yaml -n agentgram
```
  {{< /tab >}}
  {{< tab name="Container images" >}}
Pull the published images straight from GHCR.

```bash
docker pull ghcr.io/dfradehubs/agentgram-api:latest
docker pull ghcr.io/dfradehubs/agentgram-web:latest
```
  {{< /tab >}}
{{< /tabs >}}
</section>

<div class="ag-cta">
<h2 class="ag-section-title">One chat. Every agent. Any protocol.</h2>
<p class="ag-section-sub">Run the full stack locally in a couple of commands, then point it at your own agents.</p>
<div class="ag-cta-actions">
{{< hextra/hero-button text="Get Started" link="/docs/getting-started/" >}}
{{< hextra/hero-button text="View on GitHub" link="https://github.com/dfradehubs/agentgram" style="background:#27272a;border:1px solid #3f3f46;" >}}
</div>
<p class="ag-note">Open source · MIT licensed · Self-hosted</p>
</div>
