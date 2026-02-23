#!/bin/bash
#
# Demo Script: Incident Correlation with ServiceNow + Databricks
#
# This script demonstrates the SUSE AI Universal Proxy's unified MCP endpoint
# by making tool calls to both ServiceNow and Databricks through a single connection.
#
# Usage:
#   ./run-demo.sh              # Interactive demo with pauses
#   ./run-demo.sh --auto       # Automated demo (no pauses)
#
# Prerequisites:
#   - Port forward running: kubectl port-forward -n servicenow-databricks-demo svc/suse-ai-proxy 8911:8911
#   - Adapters registered: ./scripts/register-adapters.sh
#   - Demo data loaded: ./scripts/load-servicenow-data.sh --reset

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEMO_DIR="$(dirname "$SCRIPT_DIR")"

# Load .env if it exists
if [ -f "$DEMO_DIR/.env" ]; then
    set -a
    source "$DEMO_DIR/.env"
    set +a
fi

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

# Configuration
PROXY_URL="${PROXY_URL:-http://localhost:8911}"
UNIFIED_ENDPOINT="${PROXY_URL}/api/v1/mcp"
AUTO_MODE=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --auto)
            AUTO_MODE=true
            shift
            ;;
        --proxy-url)
            PROXY_URL="$2"
            UNIFIED_ENDPOINT="${PROXY_URL}/api/v1/mcp"
            shift 2
            ;;
        *)
            shift
            ;;
    esac
done

pause() {
    if [ "$AUTO_MODE" == "false" ]; then
        echo ""
        read -p "Press Enter to continue..."
        echo ""
    else
        sleep 1
    fi
}

section() {
    echo ""
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════════${NC}"
    echo -e "${BOLD}${CYAN}$1${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════════${NC}"
    echo ""
}

step() {
    echo -e "${YELLOW}→ $1${NC}"
}

result() {
    echo -e "${GREEN}$1${NC}"
}

# MCP JSON-RPC helper - uses unified endpoint
mcp_call() {
    local tool_name=$1
    local arguments=$2

    curl -s -X POST "${UNIFIED_ENDPOINT}" \
        -H "Content-Type: application/json" \
        -H "X-User-ID: admin" \
        -d "{\"jsonrpc\": \"2.0\", \"id\": 1, \"method\": \"tools/call\", \"params\": {\"name\": \"$tool_name\", \"arguments\": $arguments}}"
}

# Check services are running
check_services() {
    step "Checking proxy health..."

    if ! curl -s "$PROXY_URL/health" > /dev/null 2>&1; then
        echo -e "${RED}Universal Proxy not responding at $PROXY_URL${NC}"
        echo "Run: kubectl port-forward -n servicenow-databricks-demo svc/suse-ai-proxy 8911:8911"
        exit 1
    fi
    result "Universal Proxy: OK"

    step "Checking unified MCP endpoint..."
    TOOL_COUNT=$(curl -s -X POST "${UNIFIED_ENDPOINT}" \
        -H "Content-Type: application/json" \
        -H "X-User-ID: admin" \
        -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": {}}' | jq '.result.tools | length' 2>/dev/null || echo "0")

    if [ "$TOOL_COUNT" -gt 0 ]; then
        result "Unified endpoint: ${TOOL_COUNT} tools available"
    else
        echo -e "${RED}No tools available - are adapters registered?${NC}"
        echo "Run: ./scripts/register-adapters.sh"
        exit 1
    fi
    echo ""
}

# Demo start
clear
echo -e "${BLUE}"
cat << 'EOF'
╔══════════════════════════════════════════════════════════════════════════╗
║                                                                          ║
║    ███████╗██╗   ██╗███████╗███████╗     █████╗ ██╗                      ║
║    ██╔════╝██║   ██║██╔════╝██╔════╝    ██╔══██╗██║                      ║
║    ███████╗██║   ██║███████╗█████╗      ███████║██║                      ║
║    ╚════██║██║   ██║╚════██║██╔══╝      ██╔══██║██║                      ║
║    ███████║╚██████╔╝███████║███████╗    ██║  ██║██║                      ║
║    ╚══════╝ ╚═════╝ ╚══════╝╚══════╝    ╚═╝  ╚═╝╚═╝                      ║
║                                                                          ║
║              Universal Proxy - Unified MCP Endpoint Demo                 ║
║                     ServiceNow + Databricks                              ║
║                                                                          ║
╚══════════════════════════════════════════════════════════════════════════╝
EOF
echo -e "${NC}"

echo -e "${GREEN}Unified MCP Endpoint: ${UNIFIED_ENDPOINT}${NC}"
echo ""
echo -e "${CYAN}This demo shows how a single MCP connection gives AI assistants"
echo -e "access to multiple backend systems (ServiceNow + Databricks).${NC}"
echo ""

pause
check_services

# Scene 1: Show available tools
section "Scene 1: Unified Tool Discovery"

step "Listing all tools from unified endpoint..."
echo ""
echo -e "${CYAN}MCP Call: POST ${UNIFIED_ENDPOINT}${NC}"
echo -e "${CYAN}Method: tools/list${NC}"
echo ""

curl -s -X POST "${UNIFIED_ENDPOINT}" \
    -H "Content-Type: application/json" \
    -H "X-User-ID: admin" \
    -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": {}}' | \
    jq -r '.result.tools[] | "\(.name)"' 2>/dev/null | head -20

echo ""
echo -e "${GREEN}Note: Tools are prefixed with adapter name (servicenow__, databricks__)${NC}"

pause

# Scene 2: Search demo incidents
section "Scene 2: Search Demo Incidents (ServiceNow)"

step "Searching for demo incidents with correlation_id=SUSE-AI-DEMO..."
echo ""
echo -e "${CYAN}Tool: servicenow__search_incidents${NC}"
echo ""

RESULT=$(mcp_call "servicenow__search_incidents" '{"correlation_id": "SUSE-AI-DEMO"}')
echo "$RESULT" | jq -r '.result.content[0].text' 2>/dev/null | jq -r '.incidents[] | "  \(.number): \(.short_description) (P\(.priority))"' 2>/dev/null || echo "$RESULT"

pause

# Scene 3: Get incident details
section "Scene 3: Get Incident Details (ServiceNow)"

# Get the first incident number from demo data
INCIDENT=$(echo "$RESULT" | jq -r '.result.content[0].text' 2>/dev/null | jq -r '.incidents[0].number' 2>/dev/null || echo "INC0010014")

step "Getting details for incident ${INCIDENT}..."
echo ""
echo -e "${CYAN}Tool: servicenow__get_incident${NC}"
echo ""

RESULT=$(mcp_call "servicenow__get_incident" "{\"incident_number\": \"${INCIDENT}\"}")
echo "$RESULT" | jq -r '.result.content[0].text' 2>/dev/null | jq '{
  number: .incident.number,
  priority: .incident.priority,
  state: .incident.state,
  short_description: .incident.short_description,
  description: .incident.description
}' 2>/dev/null || echo "$RESULT" | head -50

pause

# Scene 4: Check Databricks error metrics
section "Scene 4: Correlate with Databricks Error Metrics"

step "Querying Databricks for error metrics during the incident timeframe..."
echo ""
echo -e "${CYAN}Tool: databricks__get_error_metrics${NC}"
echo ""

RESULT=$(mcp_call "databricks__get_error_metrics" '{"time_range": "last_6_hours"}')
echo "$RESULT" | jq -r '.result.content[0].text' 2>/dev/null | jq '.' || echo "$RESULT" | head -30

pause

# Scene 5: Check system health
section "Scene 5: Check System Health (Databricks)"

step "Getting current system health metrics..."
echo ""
echo -e "${CYAN}Tool: databricks__get_system_health${NC}"
echo ""

RESULT=$(mcp_call "databricks__get_system_health" '{}')
echo "$RESULT" | jq -r '.result.content[0].text' 2>/dev/null | jq '.' || echo "$RESULT" | head -30

pause

# Scene 6: Correlate events
section "Scene 6: Cross-System Event Correlation"

step "Running automated event correlation for the incident..."
echo ""
echo -e "${CYAN}Tool: databricks__correlate_events${NC}"
echo ""

RESULT=$(mcp_call "databricks__correlate_events" "{\"incident_id\": \"${INCIDENT}\"}")
echo "$RESULT" | jq -r '.result.content[0].text' 2>/dev/null | jq '.' || echo "$RESULT" | head -40

pause

# Summary
section "Demo Summary"

echo -e "${GREEN}The SUSE AI Universal Proxy demonstrated:${NC}"
echo ""
echo "  1. ${BOLD}Unified MCP Endpoint${NC}"
echo "     Single connection for all MCP servers: ${UNIFIED_ENDPOINT}"
echo ""
echo "  2. ${BOLD}Tool Aggregation${NC}"
echo "     Tools from ServiceNow + Databricks available through one interface"
echo "     Format: adapter__toolname (e.g., servicenow__search_incidents)"
echo ""
echo "  3. ${BOLD}Cross-Platform Queries${NC}"
echo "     AI can query both ITSM and analytics without switching connections"
echo ""
echo "  4. ${BOLD}Incident Correlation${NC}"
echo "     Correlated ServiceNow incident with Databricks error metrics"
echo ""

echo -e "${CYAN}For the interactive Claude Code demo, see: DEMO_SCRIPT.md${NC}"
echo ""

echo -e "${BLUE}═══════════════════════════════════════════════════════════════════${NC}"
echo -e "${BOLD}${GREEN}                    Demo Complete!                               ${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════════════${NC}"
