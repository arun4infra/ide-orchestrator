#!/bin/bash
set -euo pipefail

# Runtime Bootstrapper for ide-orchestrator service
# This script runs inside the container to start the application

# 1. Construct DATABASE_URL from platform-provided granular variables
# These are injected via envFrom: ide-orchestrator-db-conn
if [[ -n "${POSTGRES_HOST:-}" ]]; then
    export DATABASE_URL="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DB}?sslmode=disable"
    echo "🔍 Constructed DATABASE_URL for ${POSTGRES_HOST}"
    
    # Wait up to 30 seconds for the port to open (additional safety check)
    echo "🔍 Verifying database connectivity..."
    TIMEOUT=30
    while ! nc -z "$POSTGRES_HOST" "$POSTGRES_PORT" && [ $TIMEOUT -gt 0 ]; do
        echo "Waiting for database port... ($TIMEOUT seconds remaining)"
        sleep 1
        let TIMEOUT-=1
    done
    
    if [ $TIMEOUT -eq 0 ]; then
        echo "⚠️ Database port check timed out, but continuing (app will retry)"
    else
        echo "✅ Database port is open"
    fi
fi

# 2. Optional dependency wait (basic connectivity check)
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