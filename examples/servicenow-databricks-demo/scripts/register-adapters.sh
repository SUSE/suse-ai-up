#!/bin/bash
#
# Register ServiceNow and Databricks MCP servers with the Universal Proxy
#
# This script registers the MCP servers as adapters so they can be accessed
# through the Universal Proxy's unified endpoint.
#
# Usage:
#   ./register-adapters.sh
#   ./register-adapters.sh --proxy-url http://custom-proxy:8911

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

# Default URLs - using k8s service DNS names
PROXY_URL="${PROXY_URL:-http://localhost:8911}"
SERVICENOW_ENDPOINT="${SERVICENOW_ENDPOINT:-http://servicenow-mcp.servicenow-databricks-demo.svc.cluster.local:8000/mcp}"
DATABRICKS_ENDPOINT="${DATABRICKS_ENDPOINT:-http://databricks-mcp.servicenow-databricks-demo.svc.cluster.local:8001/mcp}"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --proxy-url)
            PROXY_URL="$2"
            shift 2
            ;;
        --local)
            # Use localhost endpoints (when port-forwarding all services)
            SERVICENOW_ENDPOINT="http://localhost:8000/mcp"
            DATABRICKS_ENDPOINT="http://localhost:8001/mcp"
            shift
            ;;
        *)
            shift
            ;;
    esac
done

echo -e "${BLUE}"
echo "╔══════════════════════════════════════════════════════════════════╗"
echo "║         Registering MCP Adapters with Universal Proxy            ║"
echo "╚══════════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

echo -e "${YELLOW}Proxy URL: ${PROXY_URL}${NC}"
echo ""

# Wait for proxy to be ready
echo -e "${YELLOW}Checking proxy health...${NC}"
for i in {1..30}; do
    if curl -s "${PROXY_URL}/health" > /dev/null 2>&1; then
        echo -e "${GREEN}Proxy is ready${NC}"
        break
    fi
    if [ $i -eq 30 ]; then
        echo -e "${RED}Proxy not responding at ${PROXY_URL}${NC}"
        exit 1
    fi
    sleep 1
done

echo ""

# Step 1: Register ServiceNow in the registry
echo -e "${YELLOW}Step 1: Registering ServiceNow server in registry...${NC}"
RESULT=$(curl -s -X POST "${PROXY_URL}/api/v1/registry/upload" \
  -H "Content-Type: application/json" \
  -H "X-User-ID: admin" \
  -d "{
    \"name\": \"servicenow\",
    \"type\": \"remote\",
    \"url\": \"${SERVICENOW_ENDPOINT}\",
    \"meta\": {
      \"category\": \"service-management\",
      \"sidecarConfig\": {
        \"commandType\": \"http\",
        \"command\": \"${SERVICENOW_ENDPOINT}\"
      },
      \"tags\": [\"servicenow\", \"itsm\", \"incident-management\"]
    },
    \"about\": {
      \"title\": \"ServiceNow ITSM\",
      \"description\": \"ServiceNow MCP Server for incident management, CMDB queries, and ITSM workflows.\"
    }
  }" 2>&1)

if echo "$RESULT" | grep -q '"id"' || echo "$RESULT" | grep -q '"name":"servicenow"' || echo "$RESULT" | grep -q 'already exists'; then
    echo -e "${GREEN}ServiceNow server registered in registry${NC}"
else
    echo -e "${YELLOW}Registry response: ${RESULT}${NC}"
fi

echo ""

# Step 2: Register Databricks in the registry
echo -e "${YELLOW}Step 2: Registering Databricks server in registry...${NC}"
RESULT=$(curl -s -X POST "${PROXY_URL}/api/v1/registry/upload" \
  -H "Content-Type: application/json" \
  -H "X-User-ID: admin" \
  -d "{
    \"name\": \"databricks\",
    \"type\": \"remote\",
    \"url\": \"${DATABRICKS_ENDPOINT}\",
    \"meta\": {
      \"category\": \"data-platform\",
      \"sidecarConfig\": {
        \"commandType\": \"http\",
        \"command\": \"${DATABRICKS_ENDPOINT}\"
      },
      \"tags\": [\"databricks\", \"analytics\", \"data-platform\"]
    },
    \"about\": {
      \"title\": \"Databricks\",
      \"description\": \"Databricks MCP Server for cluster management, job monitoring, SQL queries, and analytics.\"
    }
  }" 2>&1)

if echo "$RESULT" | grep -q '"id"' || echo "$RESULT" | grep -q '"name":"databricks"' || echo "$RESULT" | grep -q 'already exists'; then
    echo -e "${GREEN}Databricks server registered in registry${NC}"
else
    echo -e "${YELLOW}Registry response: ${RESULT}${NC}"
fi

echo ""

# Step 3: Create ServiceNow adapter
echo -e "${YELLOW}Step 3: Creating ServiceNow adapter...${NC}"
RESULT=$(curl -s -X POST "${PROXY_URL}/api/v1/adapters" \
  -H "Content-Type: application/json" \
  -H "X-User-ID: admin" \
  -d "{
    \"name\": \"servicenow\",
    \"mcpServerId\": \"servicenow\"
  }" 2>&1)

if echo "$RESULT" | grep -q '"name":"servicenow"' || echo "$RESULT" | grep -q '"status":"ready"' || echo "$RESULT" | grep -q 'already exists'; then
    echo -e "${GREEN}ServiceNow adapter created${NC}"
    echo "  Endpoint: ${PROXY_URL}/api/v1/adapters/servicenow/mcp"
else
    echo -e "${YELLOW}Adapter response: ${RESULT}${NC}"
fi

echo ""

# Step 4: Create Databricks adapter
echo -e "${YELLOW}Step 4: Creating Databricks adapter...${NC}"
RESULT=$(curl -s -X POST "${PROXY_URL}/api/v1/adapters" \
  -H "Content-Type: application/json" \
  -H "X-User-ID: admin" \
  -d "{
    \"name\": \"databricks\",
    \"mcpServerId\": \"databricks\"
  }" 2>&1)

if echo "$RESULT" | grep -q '"name":"databricks"' || echo "$RESULT" | grep -q '"status":"ready"' || echo "$RESULT" | grep -q 'already exists'; then
    echo -e "${GREEN}Databricks adapter created${NC}"
    echo "  Endpoint: ${PROXY_URL}/api/v1/adapters/databricks/mcp"
else
    echo -e "${YELLOW}Adapter response: ${RESULT}${NC}"
fi

echo ""

# List registered adapters
echo -e "${YELLOW}Registered adapters:${NC}"
curl -s "${PROXY_URL}/api/v1/adapters" -H "X-User-ID: admin" | jq -r '.[] | "  - \(.name): \(.status // "registered")"' 2>/dev/null || \
curl -s "${PROXY_URL}/api/v1/adapters" -H "X-User-ID: admin"

echo ""
echo -e "${GREEN}Done! Access MCP servers through the proxy:${NC}"
echo ""
echo "  ServiceNow: ${PROXY_URL}/api/v1/adapters/servicenow/mcp"
echo "  Databricks: ${PROXY_URL}/api/v1/adapters/databricks/mcp"
echo ""
echo -e "${YELLOW}Run the demo:${NC}"
echo "  ./scripts/run-demo.sh --proxy"
