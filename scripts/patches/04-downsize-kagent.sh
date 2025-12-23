#!/bin/bash
# Downsize Kagent instance for preview environments
# Reduces: medium → micro (100m-500m CPU, 256Mi-1Gi RAM)

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Determine if we're in preview mode
IS_PREVIEW_MODE=false
if kubectl get nodes -o jsonpath='{.items[0].metadata.name}' 2>/dev/null | grep -q "preview\|kind"; then
    IS_PREVIEW_MODE=true
fi

# Get repository root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

echo -e "${BLUE}🔧 Kagent Resource Optimization${NC}"
echo "Preview mode: $IS_PREVIEW_MODE"

if [ "$IS_PREVIEW_MODE" = true ]; then
    # Check if kagent is deployed and downsize it
    if kubectl get deployment kagent-controller -n intelligence-deepagents &>/dev/null; then
        echo -e "${BLUE}Downsizing kagent-controller...${NC}"
        kubectl patch deployment kagent-controller -n intelligence-deepagents --patch='
        spec:
          template:
            spec:
              containers:
              - name: controller
                resources:
                  requests:
                    cpu: "50m"
                    memory: "128Mi"
                  limits:
                    cpu: "200m"
                    memory: "256Mi"'
        echo -e "${GREEN}✓${NC} Kagent Controller: downsized to 50m-200m CPU, 128Mi-256Mi RAM"
    fi
    
    if kubectl get deployment kagent-ui -n intelligence-deepagents &>/dev/null; then
        echo -e "${BLUE}Downsizing kagent-ui...${NC}"
        kubectl patch deployment kagent-ui -n intelligence-deepagents --patch='
        spec:
          template:
            spec:
              containers:
              - name: ui
                resources:
                  requests:
                    cpu: "25m"
                    memory: "64Mi"
                  limits:
                    cpu: "100m"
                    memory: "128Mi"'
        echo -e "${GREEN}✓${NC} Kagent UI: downsized to 25m-100m CPU, 64Mi-128Mi RAM"
    fi
    
    # Scale down replicas for preview
    if kubectl get deployment kagent-controller -n intelligence-deepagents &>/dev/null; then
        kubectl scale deployment kagent-controller --replicas=0 -n intelligence-deepagents
        echo -e "${GREEN}✓${NC} Kagent Controller: scaled to 0 replicas (disabled for preview)"
    fi
    
    if kubectl get deployment kagent-ui -n intelligence-deepagents &>/dev/null; then
        kubectl scale deployment kagent-ui --replicas=0 -n intelligence-deepagents
        echo -e "${GREEN}✓${NC} Kagent UI: scaled to 0 replicas (disabled for preview)"
    fi
    
else
    echo -e "${YELLOW}⊘${NC} Not in preview mode, skipping kagent optimization"
fi

echo -e "${GREEN}✓ Kagent optimization complete${NC}"