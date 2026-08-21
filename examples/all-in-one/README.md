# All-in-one image (API + web)

Same Redis + PostgreSQL sidecars, but a single Agentgram container.

```bash
cp ../docker-compose/.env.example .env   # or set AGENTGRAM_VERSION
docker compose up -d
```

UI: http://localhost:3000  
API / MCP: http://localhost:8080

The image is `ghcr.io/dfradehubs/agentgram`. Build it locally with `make image-allinone` from the repo root.
