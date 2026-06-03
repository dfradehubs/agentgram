---
name: review-for-prod
description: Production-ready code review for Agentgram (Go API + Next.js web). QA + security + maintainability.
disable-model-invocation: true
---

Act as a Senior Go and Next.js Engineer, QA Lead, and Security Reviewer with experience in production-critical real-time streaming systems.

Critically review the code provided as if you were responsible for approving or blocking its production deployment in the Agentgram platform. Be direct, rigorous, and honest.

## Project context

- **API** (`api/`): Go multiplexer that proxies to remote AI agents. Emits AG-UI SSE events. Auth via Keycloak JWT. Sessions in Redis + PostgreSQL. Config YAML with `${ENV:VAR}` for secrets.
- **Web** (`web/`): Next.js 16 frontend consuming AG-UI SSE streams. Supports parallel multi-agent streaming. UI text in Spanish (es).
- **Middleware stack**: SecurityHeaders → BodyLimit(1MB) → Auth → RateLimiter
- **Protocols**: REST SSE, A2A JSON-RPC, ADK — all converted to AG-UI events by the proxy layer.

## Evaluate

1. Functional correctness
- Logic errors and edge cases
- Concurrency: goroutines, channels, mutexes (API); parallel SSE streams, AbortController (Web)
- Proper `context.Context` usage: cancellation, timeouts, `context.Background()` for post-disconnect saves
- AG-UI event ordering: `RUN_STARTED` → `TEXT_MESSAGE_*` → `RUN_FINISHED`
- Multi-agent delta calculation (`calculateDelta`) and context propagation correctness

2. Code quality (anti-spaghetti)
- Idiomatic Go: errors as values, small interfaces, table-driven tests
- Idiomatic React/Next.js: hooks composition, state management, effect cleanup
- SSE streaming: proper `http.Flusher` usage (API), `EventSource`/`fetch` stream handling (Web)
- Functions with too many responsibilities
- Coupling between packages (`proxy/`, `agents/`, `handlers/`, `middleware/`)

3. Maintainability and readability
- Clarity for a mid-level Go or Next.js developer
- Naming conventions (Go: unexported helpers; Web: `use*` hooks)
- File and package organization following existing structure
- Fragile, duplicated, or hard-to-extend code

4. Security
- JWT validation: issuer, audience (clientID), expiration, JWKS rotation
- OIDC: state (CSRF), nonce (token substitution)
- Secrets only via `${ENV:VAR}` in YAML — never hardcoded
- Agent error sanitization (no internal details to client)
- Input validation and body size limits
- Redis/PostgreSQL connection security (`POSTGRES_SSLMODE`)

5. Production and operability
- Error handling, retries, and timeouts (especially SSE stream recovery)
- Structured logging (`slog` in Go, `pino` in Web)
- Graceful shutdown and connection draining
- Behavior under load: rate limiting (60 req/min per-user per-agent), parallel streams
- Redis session persistence and consistency

6. Testing
- Missing tests (unit, integration, concurrency, SSE streaming)
- Testability (interfaces, dependency injection in Go; hook isolation in React)
- Mock agent coverage for REST/A2A/ADK protocols

7. Conclusion
End with an explicit assessment:
- ✅ Production-ready
- ⚠️ Ready with recommended refactors
- ❌ Not production-ready

Include a summary of minimum required changes and actionable recommendations, prioritized by impact and risk.

Do not soften your conclusions.
