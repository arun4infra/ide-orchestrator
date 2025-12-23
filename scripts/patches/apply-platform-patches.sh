#!/bin/bash
set -euo pipefail

# ==============================================================================
# Apply Platform Patches for IDE Orchestrator
# ==============================================================================
# Applies ide-orchestrator-specific patches to the platform BEFORE bootstrap
# This disables Kagent for preview mode to save CPU resources
# ==============================================================================

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $*" >&2; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $*" >&2; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }
log_warn() { echo -e "${YELLOW}[WARNING]${NC} $*" >&2; }

main() {
    log_info "Applying ide-orchestrator-specific platform patches..."
    
    # Ensure zerotouch-platform exists
    if [[ ! -d "zerotouch-platform" ]]; then
        log_error "zerotouch-platform directory not found"
        exit 1
    fi
    
    # Disable Kagent for preview mode
    KAGENT_FILE="zerotouch-platform/bootstrap/argocd/base/01-kagent.yaml"
    
    if [[ -f "$KAGENT_FILE" ]]; then
        log_info "Disabling Kagent for preview mode..."
        
        # Check if already patched
        if grep -q "# IDE Orchestrator preview patch" "$KAGENT_FILE" 2>/dev/null; then
            log_warn "Kagent already patched, skipping..."
        else
            # Use awk to insert the disable configuration after "values: |"
            awk '
                /values: \|/ {
                    print
                    print "        # IDE Orchestrator preview patch - disable Kagent to save CPU"
                    print "        agents:"
                    print "          enabled: false"
                    print "        controller:"
                    print "          enabled: false"
                    next
                }
                { print }
            ' "$KAGENT_FILE" > "$KAGENT_FILE.tmp" && mv "$KAGENT_FILE.tmp" "$KAGENT_FILE"
            
            log_success "Kagent disabled for preview mode"
            log_info "Saves ~500m CPU and ~1Gi memory"
        fi
    else
        log_error "Kagent file not found: $KAGENT_FILE"
        exit 1
    fi
    
    log_success "Platform patches applied successfully"
}

main "$@"
