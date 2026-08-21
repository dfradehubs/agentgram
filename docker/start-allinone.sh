#!/bin/sh
set -e
export CONFIG_PATH="${CONFIG_PATH:-/config/config.yaml}"
export MIGRATIONS_PATH="${MIGRATIONS_PATH:-/migrations}"
export BACKEND_URL="${BACKEND_URL:-http://127.0.0.1:8080}"
export PORT="${WEB_PORT:-3000}"
export HOSTNAME="${HOSTNAME:-0.0.0.0}"

/server &
exec node --max-http-header-size=65536 /app/server.js
