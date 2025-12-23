#!/bin/bash
# Downsize DeepAgents Runtime instance for preview environments
# Reduces: medium → small (200m-1000m CPU, 512Mi-1Gi RAM)

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

FORCE_UPDATE=false

# Parse arguments
if [ "$1" = "--force" ]; then
    FORCE_UPDATE=true
fi

# Check if this is preview mode
IS_PREVIEW_MODE=false

if [ "$FORCE_UPDATE" = true ]; then
    IS_PREVIEW_MODE=true
elif command -v kubectl > /dev/null 2>&1 && kubectl cluster-info > /dev/null 2>&1; then
    # Check if running on Kind cluster (no control-plane taints on nodes)
    if ! kubectl get nodes -o jsonpath='{.items[*].spec.taints[?(@.key=="node-role.kubernetes.io/control-plane")]}' 2>/dev/null | grep -q "control-plane"; then
        IS_PREVIEW_MODE=true
    fi
fi

if [ "$IS_PREVIEW_MODE" = true ]; then
    echo -e "${BLUE}🔧 Optimizing DeepAgents Runtime resources for preview mode...${NC}"
    
    # DeepAgents Runtime is deployed by the platform, not by ide-orchestrator
    # It should already be optimized by the platform's own patches
    # This script is a placeholder for any ide-orchestrator-specific DeepAgents configuration
    
    echo -e "  ${GREEN}✓${NC} DeepAgents Runtime: managed by platform (no ide-orchestrator-specific patches needed)"
    echo -e "${GREEN}✓ DeepAgents Runtime optimization complete${NC}"
else
    echo -e "${YELLOW}⊘${NC} Not in preview mode - skipping DeepAgents Runtime optimization"
fi

exit 0