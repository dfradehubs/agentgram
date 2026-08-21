# Agentgram all-in-one: Go API + Next.js UI in a single image.
# Compose still provides Redis + PostgreSQL (or use config.laptop.yaml for embedded stores).
#
#   docker build -t ghcr.io/dfradehubs/agentgram:local -f Dockerfile .
#
# Exposes 3000 (UI) and 8080 (API / MCP).

# --- web ---
FROM node:22-alpine AS web-deps
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --ignore-scripts && npm rebuild

FROM node:22-alpine AS web-builder
WORKDIR /web
COPY --from=web-deps /web/node_modules ./node_modules
COPY web/ ./
RUN npm run build

# --- api ---
FROM --platform=$BUILDPLATFORM golang:1.25 AS api-builder
WORKDIR /app
COPY api/go.mod api/go.sum ./
RUN go mod download
COPY api/ ./
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /server ./cmd/server

# --- runtime ---
FROM node:22-alpine
WORKDIR /app

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV CONFIG_PATH=/config/config.yaml
ENV MIGRATIONS_PATH=/migrations
ENV BACKEND_URL=http://127.0.0.1:8080

RUN addgroup --system --gid 1001 nodejs && \
    adduser --system --uid 1001 nextjs && \
    mkdir -p /config /migrations /app && \
    chown -R nextjs:nodejs /app /config /migrations

COPY --from=api-builder --chown=nextjs:nodejs /server /server
COPY --from=api-builder --chown=nextjs:nodejs /app/migrations /migrations
COPY --from=api-builder --chown=nextjs:nodejs /app/configs/config.quickstart.yaml /config/config.yaml
COPY --from=web-builder --chown=nextjs:nodejs /web/public ./public
COPY --from=web-builder --chown=nextjs:nodejs /web/.next/standalone ./
COPY --from=web-builder --chown=nextjs:nodejs /web/.next/static ./.next/static
COPY --chown=nextjs:nodejs docker/start-allinone.sh /start-allinone.sh
RUN chmod +x /start-allinone.sh /server

USER nextjs

EXPOSE 3000 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=20s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:3000/ || exit 1

CMD ["/start-allinone.sh"]
