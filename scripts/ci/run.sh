#!/bin/bash
set -euo pipefail

# Runtime Bootstrapper for ide-orchestrator service
# This script runs inside the container to start the application

# 1. Environment variable transformation (if needed)
# The platform provides standard names, map to app expectations if different
export DB_HOST="${POSTGRES_HOST:-localhost}"
export DB_PORT="${POSTGRES_PORT:-5432}"
export DB_NAME="${POSTGRES_DB:-ide_orchestrator}"
export DB_USER="${POSTGRES_USER:-postgres}"
export DB_PASSWORD="${POSTGRES_PASSWORD:-}"

# 2. Optional dependency wait (basic connectivity check)
if [[ -n "${POSTGRES_HOST:-}" ]]; then
    echo "🔍 Checking database connectivity at ${POSTGRES_HOST}:${POSTGRES_PORT}..."
    # Let the app handle connection retries - just log the attempt
fi

if [[ -n "${SPEC_ENGINE_URL:-}" ]]; then
    echo "🔍 Will connect to spec engine at ${SPEC_ENGINE_URL}"
fi

# 3. Start the application
# Use 'exec' so the app becomes PID 1 (receives SIGTERM signals correctly)
echo "🚀 Starting ide-orchestrator service..."
echo "  Port: ${PORT:-8080}"
echo "  Environment: ${GO_ENV:-production}"
echo "  Log Level: ${LOG_LEVEL:-info}"

exec ./bin/ide-orchestrator \
    --port="${PORT:-8080}" \
    --log-level="${LOG_LEVEL:-info}" \
    --env="${GO_ENV:-production}"