# Deployment examples

Recipes for running Agentgram outside your laptop. Pick the one that matches where you want to host it.

| Recipe | Best for | What you get |
| ------ | -------- | ------------ |
| [`docker-compose/`](docker-compose/) | A single VM or server | API + web + Redis + PostgreSQL, using the published images |
| [`kubernetes/`](kubernetes/) | A cluster | Helm values for the [bjw-s `app-template`](https://github.com/bjw-s/helm-charts) chart + an Ingress |

Both pull the official images from GitHub Container Registry:

```
ghcr.io/dfradehubs/agentgram-api:<version>
ghcr.io/dfradehubs/agentgram-web:<version>
```

Use a pinned version tag (e.g. `v0.1.0`) in production instead of `latest`.

## What Agentgram needs

Regardless of where you run it, the API depends on:

- **PostgreSQL** — agents, permissions, metrics and persistent data.
- **Redis** — sessions and pub/sub for live streaming.
- A **config file** (`CONFIG_PATH`) — every secret is read from the environment via `${ENV:VAR}`.

Database migrations run automatically on API startup, so there is no separate migration step.

## A note on auth

The examples ship with `auth.enabled: false` so you can try Agentgram immediately. **Do not expose an
unauthenticated instance to the public internet.** Before going live, enable Keycloak (OIDC) in the
config and put the web app behind your identity-aware proxy or Ingress. See the root
[README](../README.md#configuration) and [`api/docs/SECURITY.md`](../api/docs/SECURITY.md).
