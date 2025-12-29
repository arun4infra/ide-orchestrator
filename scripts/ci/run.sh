#!/bin/bash
set -euo pipefail

# Runtime Bootstrapper for ide-orchestrator service
# This script runs inside the container to start the application

# 1. Construct DATABASE_URL from platform-provided granular variables
# These are injected via envFrom: ide-orchestrator-db-conn
if [[ -n "${POSTGRES_HOST:-}" ]]; then
    export DATABASE_URL="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DB}?sslmode=disable"
    echo "🔍 Constructed DATABASE_URL for ${POSTGRES_HOST}"
fi

# 2. Optional dependency wait (basic connectivity check)
if [[ -n "${POSTGRES_HOST:-}" ]]; then
    echo "🔍 Checking database connectivity at ${POSTGRES_HOST}:${POSTGRES_PORT}..."
    # Let the app handle connection retries - just log the attempt
fi

if [[ -n "${IDEO_SPEC_ENGINE_URL:-}" ]]; then
    echo "🔍 Will connect to spec engine at ${IDEO_SPEC_ENGINE_URL}"
fi

# 3. Start the application
# Use 'exec' so the app becomes PID 1 (receives SIGTERM signals correctly)
echo "🚀 Starting ide-orchestrator service..."
echo "  Port: ${PORT:-8080}"
echo "  Environment: ${GO_ENV:-production}"
echo "  Log Level: ${LOG_LEVEL:-info}"

# Just exec the binary; Gin and your code will pick up PORT and GO_ENV from the environment
exec ./bin/ide-orchestrator