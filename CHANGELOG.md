# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and this project adheres to [Semantic Versioning](https://semver.org/).

## [0.9.0] - 2026-07-23

### Added

- Per-model `Max tokens` override on the Admin LLM page (`0 = auto` uses the built-in per-role default). Applied across every role — moderator, summarizer, session namer, file processor, chart extractor, and MCP chat — and reloaded without restarting Agentgram.

### Fixed

- Group debates no longer abort with "moderator returned an invalid next-speaker response: empty response" when the moderator is a reasoning/thinking model. Such models spend hidden thinking tokens before their visible answer, and the previous 64-token cap left no room for the routing decision. Moderator and synthesis token budgets were raised accordingly.


---

## [0.8.0] - 2026-07-18

### Added

- Shared LLM Providers for Anthropic, Google, OpenAI, and OpenAI-compatible Custom Endpoints, with encrypted reusable credentials and an Admin Providers page.
- Configurable group descriptions published as MCP tool purposes so clients can choose the right group.

### Changed

- LLM models now reference a Provider instead of duplicating API keys and endpoints. Existing configurations are migrated losslessly and legacy columns remain synchronized for rollback.
- Summarizer, file processor, session namer, Slack summarizer, chart extractor, and group moderator configurations resolve dynamically, so Provider edits apply without restarting Agentgram.
- Group agents receive one bounded, attributed prompt containing the original request and previous agent contributions, including with custom templates that only consume the last message.

### Security

- Custom Endpoints use SSRF-safe transports, reject redirects and unsafe endpoint forms, and omit Authorization when no API key is configured.
- Disabled Providers cannot be bypassed by selecting a model explicitly, and Providers referenced by models cannot be deleted.

## [0.7.1] - 2026-07-17

### Added

- Typing `@` in a group chat now opens an accessible, keyboard-navigable list of agents and inserts the selected agent's canonical ID.

### Changed

- Moderator configuration is resolved for each web and MCP group request, so creating, enabling, changing, or deleting the moderator model takes effect without restarting Agentgram.
- Group moderation now treats replies that lack access, evidence, tooling, or certainty as unresolved and hands the request to another suitable agent when one remains.

### Fixed

- New LLM configurations default to enabled when the field is omitted, invalid roles are rejected, explicit default models are selected deterministically, and database iteration errors are no longer ignored.

## [0.7.0] - 2026-07-17

### Added

- **Moderated agent-group debates.** Groups now use a dedicated streaming endpoint where an LLM moderator selects speakers, agents share the evolving transcript, and multi-speaker runs can end with a concise synthesis. Optional `@mentions` constrain the roster without bypassing moderation.
- **Agent groups as MCP tools.** Every accessible group is exposed as a `group__<groupId>` tool with personal session continuity and bounded debate execution.
- **Runtime administration.** Administrators can configure debate timeouts, turn limits, and MCP execution limits from the new General Configuration panel. LLM models also support a custom OpenAI-compatible endpoint and the new `moderator` role.

### Changed

- Group creation and editing moved to the admin surface. Group sessions are personal, reconnectable, and consistently attributed by agent across text, tools, charts, and error events.
- Context propagation, deadlines, persistence finalization, and public error handling are now shared and bounded consistently across web and MCP group surfaces.

### Fixed

- Prevented direct 1:1 endpoints and transport retries from bypassing or duplicating moderated group runs.
- Fixed lost or misattributed chart-only responses, stale group selection, duplicated user prompts, expired MCP save contexts, and silent moderator/session-mapping persistence failures.
- Invalid moderator output now ends as an explicit incomplete/error result instead of a false successful debate.

## [0.6.1] - 2026-06-12

### Fixed

- **Agent response header timeout raised from 60s to 5 minutes.** Agents that fan out work (e.g. querying multiple providers at once) can take >150s before emitting the first byte; the previous 60s `ResponseHeaderTimeout` in the outbound transport aborted those requests with `net/http: timeout awaiting response headers`, and each automatic retry launched a duplicate full run against the agent. Applies to all agent protocols (REST/A2A/ADK). MCP tool calls keep their own `tool_call_timeout` cap.

## [0.6.0] - 2026-06-11

### Added

- **User identity forwarded to agents and MCP servers.** Every outbound request now carries `X-User-Email` and `X-User-Groups` headers with the calling user's email and groups, taken from the validated JWT/session claims. This applies to all agent protocols (REST/A2A/ADK), all MCP server calls (tool calls and the initialize handshake), and Slack-originated chats. Header values are sanitized against injection and the groups list is capped at 4KB (whole groups dropped past the limit, never truncated mid-value).

### Security

- The new identity headers are plain, unsigned assertions: downstream services should trust them only when the caller is provably agentgram (private network and/or the bearer API key). For cryptographic proof of identity, use `auth_type: forward`, where the service receives the user's signed JWT and can verify it itself.

## [0.5.0] - 2026-06-10

### Added

- **Explicit `priority` field on API key rules** (agents and MCP servers). When a user belongs to several groups that each have a rule, the matching group rule with the **lowest priority number** wins (evaluated ascending). A user-exact rule still always wins over group rules. The admin UI exposes a per-rule priority input (disabled for user rules, where it does not apply).

### Changed

- API key rule ordering is now driven by the explicit `priority` value instead of the implicit row order. Migration `000028` renames the internal `position` column to `priority` on `agent_api_key_rules` and `mcp_api_key_rules` (existing values preserved).

## [0.4.0] - 2026-06-10

### Added

- **MCP servers now have the same outbound auth model as agents.** Bearer mode gains a configurable auth header (`auth_header_name` — `Authorization` with `Bearer ` prefix by default, or any custom header such as `X-API-Key` verbatim) and **per user/group API key rules** (`api_key_rules`), resolved with precedence exact-user > first matching group (ordered) > the server's fallback key. agentgram can now authenticate to an MCP server as a service while sending a different key per calling user or group.
- Admin UI: the MCP form bearer section gains the auth header field and a per user/group rules editor (the rules editor component is now shared between the agent and MCP forms).

### Changed

- Bearer MCP servers configured with `api_key_rules` initialize lazily per user (like forward/oauth2 servers), so each request carries the calling user's resolved key. Static bearer servers (no rules) keep eager initialization with the shared token. API clients that omit `api_key_rules` on update do not wipe existing rules.

## [0.3.0] - 2026-06-10

### Added

- **Per-agent outbound authentication method**, mirroring the MCP server auth model. Agents now have an `auth_type` (`none` | `forward` | `bearer`; `oauth2` reserved for a future release) replacing the lone forward-authorization checkbox. In **bearer** mode, agentgram authenticates to the agent as a service with an API key sent on a **configurable header** (`Authorization` with `Bearer ` prefix by default, or any custom header such as `X-API-Key` verbatim).
- **Per user/group API key rules** (bearer mode): map a user email or group to a specific API key, with precedence exact-user > first matching group (ordered) > agent-level fallback key. This lets a gateway deployment authenticate to upstream agents with service keys while preserving per-user identity, instead of forwarding the user's JWT across token audiences/issuers the agent may not trust.
- Admin UI: authentication type selector on the agent form with a conditional bearer section (fallback key, auth header, and a rules editor).

### Compatibility

- Existing agents keep working: `forward_authorization: true` still means forward (`GetAuthType` fallback), the legacy flag stays in sync, and API clients that omit `api_key_rules` on update do not wipe existing rules.

## [0.2.3] - 2026-06-10

### Fixed

- MCP OAuth metadata now advertises a configurable set of extra scopes (`mcp_server.extra_scopes`) on top of the required base set, surfaced in both `oauth-protected-resource` (RFC 9728) and `oauth-authorization-server` (RFC 8414). Strict MCP clients request only the scopes advertised in the metadata, so an audience-mapper client scope was never requested and the upstream agent rejected the forwarded token with `401`. Deployments that enforce a token audience on the agent can now advertise their audience scope and have clients include it, while the gateway keeps a curated base scope set instead of exposing every realm scope.

## [0.2.2] - 2026-06-03

### Fixed

- Scroll-up during a streaming response now works. The scroll listeners were attached in a mount-time effect that captured a null container (the messages container renders conditionally), so they were never bound and the user's scroll could not detach auto-scroll. They are now attached via a callback ref when the container actually mounts.
- Switching agent and returning during an active run no longer corrupts the message (it used to show only the tail, mislabelled as "thinking"). Single-agent runs are buffered server-side, so the browser no longer resumes from the partial background reader — it reconnects via the same full-replay path used after a reload.

## [0.2.1] - 2026-06-03

### Fixed

- Live reconnect after a real page reload (F5) now works. Two bugs prevented it: the recovery effect ran before the restored session's messages had loaded and never retried (messages was not a dependency), and the per-session run-event buffer was reused across runs, so the replay hit a previous run's `RUN_FINISHED` and closed before the current run's events. The buffer is now reset at run start and recovery fires once messages arrive.

## [0.2.0] - 2026-06-03

### Added

- **Live reconnect to an in-flight run after a page reload.** Runs already survive a client disconnect on the server; now every AG-UI event is buffered in a Redis stream (`run_events:{sessionId}`, 10-min TTL) and a new endpoint `GET /api/agents/{agentId}/sessions/{sessionId}/stream` replays what was written and continues streaming live until the run finishes. The web client detects an active run on load and reconnects, so the agent's reply appears and keeps flowing token by token instead of only showing up when complete. Falls back to session polling when there is no active run to reconnect to. Applies to 1:1 agent chat; MCP and group sessions keep their existing behaviour.

## [0.1.3] - 2026-06-03

### Fixed

- Streaming responses now render incrementally (token by token) again. Next.js gzip compression was buffering `text/event-stream` responses and flushing them in large blocks, so the agent's reply only appeared once it was fully written. Compression is now disabled in Next (`compress: false`); static asset compression should be handled at the edge (ingress/CDN/service mesh).
- Reloading the page during an active run no longer fires a duplicate run. The backend keeps the run alive and persists the full reply when it finishes, so the client now polls the session for that reply and shows it once the run completes, only re-sending as a last resort if nothing arrives.

## [0.1.2] - 2026-06-03

### Fixed

- Chat auto-scroll robustness: the previous fix did not fully release the viewport while an agent was streaming. Reworked it into a pin-to-bottom model that distinguishes the app's own programmatic scroll from the user's, so scrolling up during streaming now reliably keeps the viewport in place and auto-scroll only re-engages when the user returns to the bottom or sends a new message.

## [0.1.1] - 2026-06-03

### Fixed

- Chat auto-scroll no longer traps the viewport at the bottom while an agent is streaming a response. Scrolling up now reliably yields control (detected via wheel/touch events), and auto-scroll only re-engages when the user returns to the bottom.

## [0.1.0] - 2026-06-03

### Added

- Initial public release.
