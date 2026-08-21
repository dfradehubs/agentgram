---
title: Configuration
weight: 4
---

The API is configured from a single YAML file, pointed to by the `CONFIG_PATH` environment variable
(for example `api/configs/config.yaml`). It covers the server, auth, Redis, PostgreSQL, metrics,
tracing and the MCP server.

## Authentication

```yaml
auth:
  enabled: true          # set false only for local / trusted networks
  # Password login — no IdP required
  basic:
    enabled: true
    seed_users:
      - username: admin
        email: admin@localhost
        password: "${ENV:BASIC_AUTH_PASSWORD}"
  # Any OIDC provider (Keycloak, Authentik, Auth0, Zitadel, Google, …).
  # Endpoints are discovered from {issuer}/.well-known/openid-configuration.
  # Keycloak-style paths are used only if discovery is unreachable.
  keycloak:
    enabled: true
    display_name: "OIDC"
    issuer: "${ENV:OIDC_ISSUER}"
    jwks_cache_ttl: 3600
    client_id: "${ENV:OIDC_CLIENT_ID}"
    client_secret: "${ENV:OIDC_CLIENT_SECRET}"
    redirect_uri: "${ENV:OIDC_REDIRECT_URI}"
    post_logout_uri: "${ENV:OIDC_POST_LOGOUT_URI}"
```

When auth is enabled the API validates the JWT signature (via JWKS), issuer, audience and
expiration. The OIDC login flow additionally validates `state` (CSRF) and `nonce`.

{{< callout type="warning" >}}
Running with `auth.enabled: false` or `POSTGRES_SSLMODE: disable` is fine for local development, but
never for an instance reachable from the internet.
{{< /callout >}}

## Secrets

Secrets are **never** written inline. Every sensitive value uses the `${ENV:VAR}` syntax and is
resolved from the environment at startup:

```yaml
auth:
  keycloak:
    enabled: true
    issuer: "${ENV:KEYCLOAK_ISSUER}"
    client_id: "${ENV:OIDC_CLIENT_ID}"
    client_secret: "${ENV:OIDC_CLIENT_SECRET}"

redis:
  addr: "${ENV:REDIS_ADDR}"
  password: "${ENV:REDIS_PASSWORD}"

database:
  host: "${ENV:POSTGRES_HOST}"
  port: 5432
  user: "${ENV:POSTGRES_USER}"
  password: "${ENV:POSTGRES_PASSWORD}"
  dbname: "${ENV:POSTGRES_DB}"
  sslmode: "${ENV:POSTGRES_SSLMODE}"
```

See [`api/.env.example`](https://github.com/dfradehubs/agentgram/blob/main/api/.env.example) for the
full list of variables.

## Security hardening

The API ships with a defensive middleware stack: security headers, a 1 MB body limit, JWT/OIDC auth,
and per-user, per-agent rate limiting. Agent errors are sanitized before reaching the client. For
the full checklist see
## Laptop mode

`api/configs/config.laptop.yaml` starts **embedded Redis** (`redis.embedded: true`) and **embedded
PostgreSQL** (`database.embedded: true`) so you can run the API as one process without Docker for
datastores. Data lives under `AGENTGRAM_DATA_DIR` (default `./data`).

A SQLite dialect rewrite of the migrations lives in `api/internal/db/sqlite.go` for a future
`database/sql` repository layer; today's application code still speaks PostgreSQL (including the
embedded engine).

If you serve a static UI from the API, set `server.web_static_dir` or `WEB_STATIC_DIR`.
