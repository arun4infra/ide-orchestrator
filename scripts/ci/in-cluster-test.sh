#!/bin/bash
set -euo pipefail

# ==============================================================================
# Local CI Testing Script for ide-orchestrator
# ==============================================================================
# Purpose: Local testing of CI workflow using platform's centralized script
# Usage: ./scripts/ci/in-cluster-test.sh
# 
# This script follows the filesystem contract approach where the service
# declares requirements in ci/config.yaml and the platform handles execution.
# ==============================================================================

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[LOCAL-CI]${NC} $*"; }
log_success() { echo -e "${GREEN}[LOCAL-CI]${NC} $*"; }
log_error() { echo -e "${RED}[LOCAL-CI]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[LOCAL-CI]${NC} $*"; }

# Get script directory and service root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVICE_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

main() {
    echo "================================================================================"
    echo "Local CI Testing for ide-orchestrator"
    echo "================================================================================"
    echo "  Service Root: $SERVICE_ROOT"
    echo "  Using filesystem contract (ci/config.yaml)"
    echo "================================================================================"
    
    # Change to service root directory
    cd "$SERVICE_ROOT"
    
    # Validate filesystem contract
    validate_filesystem_contract
    
    # Clone platform if needed
    setup_platform_repository
    
    # Run centralized platform script
    run_platform_ci_script
}

validate_filesystem_contract() {
    log_info "Validating filesystem contract..."
    
    # Required files
    if [[ ! -f "ci/config.yaml" ]]; then
        log_error "Required ci/config.yaml not found"
        log_error "Service must follow filesystem contract. See platform documentation."
        exit 1
    fi
    
    # Load service name for validation
    if command -v yq &> /dev/null; then
        SERVICE_NAME=$(yq eval '.service.name' ci/config.yaml 2>/dev/null || echo "")
        NAMESPACE=$(yq eval '.service.namespace' ci/config.yaml 2>/dev/null || echo "")
    else
        log_warn "yq not found, skipping config validation"
        SERVICE_NAME="ide-orchestrator"
        NAMESPACE="intelligence-orchestrator"
    fi
    
    if [[ -z "$SERVICE_NAME" ]]; then
        log_error "service.name is required in ci/config.yaml"
        exit 1
    fi
    
    if [[ -z "$NAMESPACE" ]]; then
        log_error "service.namespace is required in ci/config.yaml"
        exit 1
    fi
    
    log_success "Filesystem contract validated for service: $SERVICE_NAME"
    
    # Optional structure validation
    if [[ -d "migrations" ]]; then
        log_info "Found migrations/ directory"
    fi
    
    if [[ -d "tests/integration" ]]; then
        log_info "Found tests/integration/ directory"
    fi
    
    if [[ -d "patches" ]]; then
        log_info "Found patches/ directory"
    fi
}

setup_platform_repository() {
    log_info "Setting up platform repository..."
    
    # Determine platform branch from config or use main
    PLATFORM_BRANCH="main"
    if command -v yq &> /dev/null; then
        PLATFORM_BRANCH=$(yq eval '.platform.branch // "main"' ci/config.yaml 2>/dev/null)
    fi
    
    # Create folder name with branch (replace / with -)
    BRANCH_FOLDER_NAME=$(echo "$PLATFORM_BRANCH" | sed 's/\//-/g')
    PLATFORM_CHECKOUT_DIR="zerotouch-platform-${BRANCH_FOLDER_NAME}"
    
    if [[ -d "$PLATFORM_CHECKOUT_DIR" ]]; then
        log_info "Platform directory exists ($PLATFORM_CHECKOUT_DIR), updating..."
        cd "$PLATFORM_CHECKOUT_DIR"
        git fetch origin
        git checkout "$PLATFORM_BRANCH"
        git pull origin "$PLATFORM_BRANCH"
        cd - > /dev/null
    else
        log_info "Cloning zerotouch-platform repository (branch: $PLATFORM_BRANCH)..."
        git clone -b "$PLATFORM_BRANCH" https://github.com/arun4infra/zerotouch-platform.git "$PLATFORM_CHECKOUT_DIR"
    fi
    
    log_success "Platform repository ready at: $PLATFORM_CHECKOUT_DIR (branch: $PLATFORM_BRANCH)"
}

run_platform_ci_script() {
    log_info "Running centralized platform CI script..."
    
    # Path to centralized script
    PLATFORM_SCRIPT="${PLATFORM_CHECKOUT_DIR}/scripts/bootstrap/preview/tenants/scripts/in-cluster-test.sh"
    
    if [[ ! -f "$PLATFORM_SCRIPT" ]]; then
        log_error "Platform CI script not found: $PLATFORM_SCRIPT"
        log_error "Ensure zerotouch-platform repository is properly cloned"
        exit 1
    fi
    
    # Make script executable
    chmod +x "$PLATFORM_SCRIPT"
    
    # Run the centralized script (no arguments - uses filesystem contract)
    log_info "Executing: $PLATFORM_SCRIPT"
    echo ""
    
    if "$PLATFORM_SCRIPT"; then
        echo ""
        log_success "✅ Local CI testing completed successfully!"
        echo ""
        echo "================================================================================"
        echo "LOCAL CI TESTING COMPLETE"
        echo "================================================================================"
        echo "  Service: ide-orchestrator"
        echo "  Result:  PASSED"
        echo "  Method:  Filesystem Contract (ci/config.yaml)"
        echo ""
        echo "The service is ready for CI/CD pipeline execution."
        echo "================================================================================"
    else
        echo ""
        log_error "❌ Local CI testing failed!"
        echo ""
        echo "================================================================================"
        echo "LOCAL CI TESTING FAILED"
        echo "================================================================================"
        echo "  Service: ide-orchestrator"
        echo "  Result:  FAILED"
        echo ""
        echo "Check the logs above for specific failure details."
        echo "Fix issues and re-run this script before pushing to CI/CD."
        echo "================================================================================"
        exit 1
    fi
}

# Cleanup function
cleanup() {
    log_info "Cleaning up local CI testing..."
    
    # Clean up Kind cluster if it exists
    if command -v kind &> /dev/null; then
        kind delete cluster --name zerotouch-preview 2>/dev/null || true
    fi
    
    log_info "Local CI cleanup completed"
}

# Error handler
error_handler() {
    local exit_code=$?
    local line_number=$1
    log_error "Script failed at line $line_number with exit code $exit_code"
    log_error "Last command: $BASH_COMMAND"
    cleanup
    exit $exit_code
}

# Set up error handling
trap 'error_handler $LINENO' ERR
trap cleanup EXIT

# Run main function
main "$@"