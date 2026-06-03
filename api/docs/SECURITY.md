# Security

## Middleware Stack

All requests pass through these middlewares (in order):

1. **RequestID** - Unique request identifier
2. **RealIP** - Client IP extraction from proxy headers
3. **SecurityHeaders** - HTTP security headers on all responses
4. **BodyLimit (1MB)** - Prevents oversized request bodies (DoS protection)
5. **Logging** - Structured request/response logging
6. **Recoverer** - Panic recovery
7. **Auth** - JWT/OIDC session validation (on `/api/*` routes)
8. **RateLimiter** - Per-user per-agent rate limiting on chat endpoints

## HTTP Security Headers

Applied globally via `middleware/security.go`:

| Header | Value | Purpose |
|--------|-------|---------|
| X-Content-Type-Options | nosniff | Prevent MIME sniffing |
| X-Frame-Options | DENY | Prevent clickjacking |
| X-XSS-Protection | 0 | Disable legacy XSS filter |
| Referrer-Policy | strict-origin-when-cross-origin | Limit referrer leakage |
| Strict-Transport-Security | max-age=31536000; includeSubDomains | Force HTTPS |
| Permissions-Policy | camera=(), microphone=(), ... | Restrict browser APIs |

The web (Next.js) applies the same headers via `next.config.ts`.

## Authentication

### JWT Bearer Tokens
- Validated via JWKS from Keycloak (RS256)
- Claims checked: **issuer**, **audience** (clientID), **expiration**
- Extracted: email, sub, groups (from multiple token locations)

### OIDC Session Flow
1. Login generates **state** (CSRF) and **nonce** (token substitution prevention)
2. State stored in Redis with 5-minute TTL
3. Callback validates state, exchanges code for tokens
4. ID token validated: signature, issuer, audience, **nonce** (matched against stored value)
5. Session created with crypto/rand 256-bit ID, stored server-side in Redis

### Cookie Security
- `HttpOnly: true` - No JavaScript access
- `Secure: true` (production) - HTTPS only
- `SameSite: Lax` - CSRF protection
- `Path: /` - All routes

## Rate Limiting

- **Chat endpoints**: 60 requests/minute per user per agent
- Implementation: in-memory token bucket (`golang.org/x/time/rate`)
- Burst: 10% of rate (6 requests)
- Returns `429 Too Many Requests` with `Retry-After: 60` header

## Database Security

- **SSL mode**: Configurable via `POSTGRES_SSLMODE` env var (default: `require`)
- **Connection string**: Uses parameterized format (no SQL injection)
- **All queries**: Use parameterized queries (`$1`, `$2`, etc.)

## Agent Communication

- Agent errors are **sanitized** before sending to clients - only status code is exposed
- Full error details are logged server-side for debugging
- Agent responses are proxied through SSE writer (no raw forwarding)

## Environment Variables for Security

| Variable | Purpose | Default |
|----------|---------|---------|
| `POSTGRES_SSLMODE` | Database SSL mode | `require` |
| `LOG_LEVEL` | Logging verbosity | `info` |
| `OIDC_CLIENT_ID` | Keycloak client ID (audience validation) | required |
| `OIDC_CLIENT_SECRET` | Keycloak client secret | required |

## Known Limitations

> **Warning**: These items should be addressed before exposing the API to untrusted networks.

- Rate limiter is in-memory (not distributed) - resets on restart, doesn't sync across instances. Consider Redis-backed rate limiting for multi-instance deployments.
- LLM API keys stored as plaintext in PostgreSQL (encryption at rest via infrastructure). Consider application-level encryption for sensitive credentials.
- No JTI-based JWT replay protection (tokens valid until expiration). Short-lived tokens and token rotation mitigate this risk.
