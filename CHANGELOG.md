# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and this project adheres to [Semantic Versioning](https://semver.org/).

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
