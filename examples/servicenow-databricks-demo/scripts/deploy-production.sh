#!/bin/bash
#
# Deploy MCP servers in production mode (real ServiceNow + Databricks)
#
# Prerequisites:
#   - .env file configured with real credentials
#   - Demo data loaded in ServiceNow and Databricks
#   - Kubernetes cluster running (Rancher Desktop or RKE2)
#
# Usage:
#   ./scripts/deploy-production.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Load .env if it exists
if [ -f "$PROJECT_DIR/.env" ]; then
    set -a
    source "$PROJECT_DIR/.env"
    set +a
fi

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}"
echo "╔══════════════════════════════════════════════════════════════════╗"
echo "║         Deploying MCP Servers in Production Mode                 ║"
echo "╚══════════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

# Check prerequisites
echo -e "${YELLOW}Checking prerequisites...${NC}"

if [ -z "$SERVICENOW_INSTANCE_URL" ]; then
    echo -e "${RED}Error: SERVICENOW_INSTANCE_URL not set. Configure .env first.${NC}"
    exit 1
fi

if [ -z "$DATABRICKS_HOST" ]; then
    echo -e "${RED}Error: DATABRICKS_HOST not set. Configure .env first.${NC}"
    exit 1
fi

if ! kubectl cluster-info > /dev/null 2>&1; then
    echo -e "${RED}Error: Cannot connect to Kubernetes cluster${NC}"
    exit 1
fi

echo -e "${GREEN}Prerequisites OK${NC}"
echo ""

# Create namespace if it doesn't exist
echo -e "${YELLOW}Ensuring namespace exists...${NC}"
kubectl create namespace servicenow-databricks-demo --dry-run=client -o yaml | kubectl apply -f -
echo ""

# Create/update secrets
echo -e "${YELLOW}Creating Kubernetes secrets...${NC}"

# Delete existing secrets (to update them)
kubectl delete secret servicenow-credentials -n servicenow-databricks-demo 2>/dev/null || true
kubectl delete secret databricks-credentials -n servicenow-databricks-demo 2>/dev/null || true

# Create ServiceNow secret
kubectl create secret generic servicenow-credentials \
    --namespace servicenow-databricks-demo \
    --from-literal=SERVICENOW_INSTANCE_URL="${SERVICENOW_INSTANCE_URL}" \
    --from-literal=SERVICENOW_USERNAME="${SERVICENOW_USERNAME}" \
    --from-literal=SERVICENOW_PASSWORD="${SERVICENOW_PASSWORD}" \
    --from-literal=SERVICENOW_AUTH_METHOD="${SERVICENOW_AUTH_METHOD:-basic}" \
    --from-literal=SERVICENOW_PAT="${SERVICENOW_PAT:-}" \
    --from-literal=SERVICENOW_CLIENT_ID="${SERVICENOW_CLIENT_ID:-}" \
    --from-literal=SERVICENOW_CLIENT_SECRET="${SERVICENOW_CLIENT_SECRET:-}"

echo -e "${GREEN}  ServiceNow credentials created${NC}"

# Create Databricks secret
kubectl create secret generic databricks-credentials \
    --namespace servicenow-databricks-demo \
    --from-literal=DATABRICKS_HOST="${DATABRICKS_HOST}" \
    --from-literal=DATABRICKS_TOKEN="${DATABRICKS_TOKEN}" \
    --from-literal=DATABRICKS_WAREHOUSE_ID="${DATABRICKS_WAREHOUSE_ID}" \
    --from-literal=DATABRICKS_CATALOG="${DATABRICKS_CATALOG:-workspace}" \
    --from-literal=DATABRICKS_SCHEMA="${DATABRICKS_SCHEMA:-analytics}"

echo -e "${GREEN}  Databricks credentials created${NC}"
echo ""

# Delete existing deployments (selectors are immutable)
echo -e "${YELLOW}Removing existing deployments...${NC}"
kubectl delete deployment servicenow-mcp databricks-mcp suse-ai-proxy -n servicenow-databricks-demo 2>/dev/null || true

# Apply base configuration
echo -e "${YELLOW}Applying base configuration...${NC}"
kubectl apply -k "$PROJECT_DIR/k8s/"

# Patch configmap for production mode
echo -e "${YELLOW}Patching configmap for production mode...${NC}"
kubectl patch configmap demo-config -n servicenow-databricks-demo --type merge -p '{
  "data": {
    "SERVICENOW_MOCK_MODE": "false",
    "DATABRICKS_MOCK_MODE": "false",
    "SERVICENOW_AUTH_METHOD": "basic"
  }
}'

# Patch databricks deployment to add warehouse/catalog/schema env vars
echo -e "${YELLOW}Patching Databricks deployment...${NC}"
kubectl patch deployment databricks-mcp -n servicenow-databricks-demo --type json -p '[
  {"op": "add", "path": "/spec/template/spec/containers/0/env/-", "value": {"name": "DATABRICKS_WAREHOUSE_ID", "valueFrom": {"secretKeyRef": {"name": "databricks-credentials", "key": "DATABRICKS_WAREHOUSE_ID"}}}},
  {"op": "add", "path": "/spec/template/spec/containers/0/env/-", "value": {"name": "DATABRICKS_CATALOG", "valueFrom": {"secretKeyRef": {"name": "databricks-credentials", "key": "DATABRICKS_CATALOG"}}}},
  {"op": "add", "path": "/spec/template/spec/containers/0/env/-", "value": {"name": "DATABRICKS_SCHEMA", "valueFrom": {"secretKeyRef": {"name": "databricks-credentials", "key": "DATABRICKS_SCHEMA"}}}}
]'
echo ""

# Wait for deployments to be ready
echo -e "${YELLOW}Waiting for deployments to be ready...${NC}"
kubectl rollout status deployment/servicenow-mcp -n servicenow-databricks-demo --timeout=120s
kubectl rollout status deployment/databricks-mcp -n servicenow-databricks-demo --timeout=120s
kubectl rollout status deployment/suse-ai-proxy -n servicenow-databricks-demo --timeout=120s
echo ""

# Show pod status
echo -e "${YELLOW}Pod status:${NC}"
kubectl get pods -n servicenow-databricks-demo
echo ""

echo -e "${GREEN}╔══════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║         Production deployment complete!                          ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo "Next steps:"
echo ""
echo "  1. Port forward the proxy:"
echo "     kubectl port-forward -n servicenow-databricks-demo svc/suse-ai-proxy 8911:8911"
echo ""
echo "  2. Register the adapters:"
echo "     ./scripts/register-adapters.sh"
echo ""
echo "  3. Configure Claude Code (see claude-code-mcp-config.json)"
echo ""
