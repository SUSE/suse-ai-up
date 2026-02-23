#!/bin/bash
#
# Load demo incident data into ServiceNow
#
# This creates incidents that correlate with Databricks error data
# for the incident correlation demo.
#
# Prerequisites:
#   - ServiceNow developer instance
#   - Personal Access Token (PAT) or Basic Auth credentials
#
# Usage:
#   export SERVICENOW_INSTANCE_URL="https://devXXXXX.service-now.com"
#   export SERVICENOW_PAT="your-pat-token"
#   ./load-servicenow-data.sh
#
#   Or with Basic Auth:
#   export SERVICENOW_USERNAME="admin"
#   export SERVICENOW_PASSWORD="password"
#   ./load-servicenow-data.sh
#
#   To reset (delete existing demo incidents first):
#   ./load-servicenow-data.sh --reset

set -e

# Parse arguments
RESET_MODE=false
while [[ $# -gt 0 ]]; do
    case $1 in
        --reset|-r)
            RESET_MODE=true
            shift
            ;;
        *)
            shift
            ;;
    esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Load .env if it exists
if [ -f "$PROJECT_DIR/.env" ]; then
    echo "Loading configuration from .env..."
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

# Configuration
INSTANCE_URL="${SERVICENOW_INSTANCE_URL:-}"
PAT="${SERVICENOW_PAT:-}"
USERNAME="${SERVICENOW_USERNAME:-}"
PASSWORD="${SERVICENOW_PASSWORD:-}"

echo -e "${BLUE}"
echo "╔══════════════════════════════════════════════════════════════════╗"
echo "║         Loading Demo Data into ServiceNow                        ║"
echo "╚══════════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

if [ "$RESET_MODE" = true ]; then
    echo -e "${YELLOW}Reset mode: Will delete existing demo incidents first${NC}"
    echo ""
fi

# Validate configuration
if [ -z "$INSTANCE_URL" ]; then
    echo -e "${RED}Error: SERVICENOW_INSTANCE_URL is required${NC}"
    echo ""
    echo "Usage:"
    echo "  export SERVICENOW_INSTANCE_URL=\"https://devXXXXX.service-now.com\""
    echo "  export SERVICENOW_PAT=\"your-pat-token\""
    echo "  ./load-servicenow-data.sh"
    exit 1
fi

# Remove trailing slash
INSTANCE_URL="${INSTANCE_URL%/}"

# Set up authentication based on AUTH_METHOD
AUTH_METHOD="${SERVICENOW_AUTH_METHOD:-pat}"

if [ "$AUTH_METHOD" = "basic" ] && [ -n "$USERNAME" ] && [ -n "$PASSWORD" ]; then
    AUTH_HEADER="Authorization: Basic $(echo -n "${USERNAME}:${PASSWORD}" | base64)"
    echo -e "${YELLOW}Using Basic authentication${NC}"
elif [ "$AUTH_METHOD" = "pat" ] && [ -n "$PAT" ]; then
    AUTH_HEADER="Authorization: Bearer ${PAT}"
    echo -e "${YELLOW}Using PAT authentication${NC}"
elif [ -n "$USERNAME" ] && [ -n "$PASSWORD" ]; then
    AUTH_HEADER="Authorization: Basic $(echo -n "${USERNAME}:${PASSWORD}" | base64)"
    echo -e "${YELLOW}Using Basic authentication${NC}"
elif [ -n "$PAT" ]; then
    AUTH_HEADER="Authorization: Bearer ${PAT}"
    echo -e "${YELLOW}Using PAT authentication${NC}"
else
    echo -e "${RED}Error: No authentication credentials provided${NC}"
    echo "Set either SERVICENOW_PAT or SERVICENOW_USERNAME/SERVICENOW_PASSWORD"
    exit 1
fi

echo -e "${YELLOW}Instance: ${INSTANCE_URL}${NC}"
echo ""

# Test connection
echo -e "${YELLOW}Testing connection...${NC}"
RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" \
    -H "${AUTH_HEADER}" \
    -H "Content-Type: application/json" \
    "${INSTANCE_URL}/api/now/table/incident?sysparm_limit=1")

if [ "$RESPONSE" != "200" ]; then
    echo -e "${RED}Failed to connect to ServiceNow (HTTP ${RESPONSE})${NC}"
    echo "Check your credentials and instance URL"
    exit 1
fi
echo -e "${GREEN}Connection successful${NC}"
echo ""

# Demo incident identifier - used for filtering and reset
DEMO_CORRELATION_ID="SUSE-AI-DEMO"

# Demo incident short descriptions (used for reset matching)
DEMO_INCIDENTS=(
    "Database connection timeout on production cluster"
    "Databricks cluster auto-scaling failure"
    "ETL pipeline data quality alert"
    "ML model prediction accuracy degradation"
)

# Function to delete demo incidents
delete_demo_incidents() {
    echo -e "${YELLOW}Searching for existing demo incidents (correlation_id=${DEMO_CORRELATION_ID})...${NC}"

    local deleted_count=0

    # First, try to find incidents by correlation_id (new method)
    local SEARCH_RESULT=$(curl -s \
        -H "${AUTH_HEADER}" \
        -H "Content-Type: application/json" \
        "${INSTANCE_URL}/api/now/table/incident?sysparm_query=correlation_id=${DEMO_CORRELATION_ID}&sysparm_fields=sys_id,number,short_description")

    # Extract sys_ids using Python
    local SYS_IDS=$(echo "$SEARCH_RESULT" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    results = data.get('result', [])
    for r in results:
        print(r.get('sys_id', ''), r.get('number', ''))
except:
    pass
" 2>/dev/null)

    # Delete each found incident
    while IFS=' ' read -r sys_id inc_number; do
        if [ -n "$sys_id" ]; then
            echo -e "  ${YELLOW}Deleting: ${inc_number}${NC}"
            local DELETE_RESULT=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
                -H "${AUTH_HEADER}" \
                "${INSTANCE_URL}/api/now/table/incident/${sys_id}")

            if [ "$DELETE_RESULT" = "204" ] || [ "$DELETE_RESULT" = "200" ]; then
                echo -e "  ${GREEN}Deleted: ${inc_number}${NC}"
                ((deleted_count++))
            else
                echo -e "  ${RED}Failed to delete ${inc_number} (HTTP ${DELETE_RESULT})${NC}"
            fi
        fi
    done <<< "$SYS_IDS"

    # Fallback: also search by short_description for older incidents without correlation_id
    if [ $deleted_count -eq 0 ]; then
        echo -e "${YELLOW}Checking for legacy demo incidents by description...${NC}"
        for desc in "${DEMO_INCIDENTS[@]}"; do
            local encoded_desc=$(python3 -c "import urllib.parse; print(urllib.parse.quote('''${desc}'''))")

            SEARCH_RESULT=$(curl -s \
                -H "${AUTH_HEADER}" \
                -H "Content-Type: application/json" \
                "${INSTANCE_URL}/api/now/table/incident?sysparm_query=short_description=${encoded_desc}&sysparm_fields=sys_id,number,short_description")

            SYS_IDS=$(echo "$SEARCH_RESULT" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    results = data.get('result', [])
    for r in results:
        print(r.get('sys_id', ''), r.get('number', ''))
except:
    pass
" 2>/dev/null)

            while IFS=' ' read -r sys_id inc_number; do
                if [ -n "$sys_id" ]; then
                    echo -e "  ${YELLOW}Deleting: ${inc_number}${NC}"
                    DELETE_RESULT=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
                        -H "${AUTH_HEADER}" \
                        "${INSTANCE_URL}/api/now/table/incident/${sys_id}")

                    if [ "$DELETE_RESULT" = "204" ] || [ "$DELETE_RESULT" = "200" ]; then
                        echo -e "  ${GREEN}Deleted: ${inc_number}${NC}"
                        ((deleted_count++))
                    else
                        echo -e "  ${RED}Failed to delete ${inc_number} (HTTP ${DELETE_RESULT})${NC}"
                    fi
                fi
            done <<< "$SYS_IDS"
        done
    fi

    if [ $deleted_count -gt 0 ]; then
        echo -e "${GREEN}Deleted ${deleted_count} demo incident(s)${NC}"
    else
        echo -e "${YELLOW}No existing demo incidents found${NC}"
    fi
    echo ""
}

# Reset mode: delete existing demo incidents first
if [ "$RESET_MODE" = true ]; then
    echo -e "${BLUE}Reset mode enabled - deleting existing demo incidents...${NC}"
    echo ""
    delete_demo_incidents
fi

# Function to create an incident using a temp file for JSON
create_incident() {
    local short_desc="$1"
    local description="$2"
    local priority="$3"
    local category="$4"
    local subcategory="$5"
    local urgency="$6"
    local impact="$7"

    echo -e "${YELLOW}Creating: ${short_desc}${NC}"

    # Create JSON using Python to handle escaping properly
    local json_payload=$(python3 -c "
import json
print(json.dumps({
    'short_description': '''${short_desc}''',
    'description': '''${description}''',
    'priority': '${priority}',
    'category': '${category}',
    'subcategory': '${subcategory}',
    'urgency': '${urgency}',
    'impact': '${impact}'
}))
")

    RESULT=$(curl -s -X POST \
        -H "${AUTH_HEADER}" \
        -H "Content-Type: application/json" \
        -H "Accept: application/json" \
        "${INSTANCE_URL}/api/now/table/incident" \
        -d "${json_payload}")

    INC_NUMBER=$(echo "$RESULT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('result',{}).get('number','UNKNOWN'))" 2>/dev/null || echo "UNKNOWN")

    if [ "$INC_NUMBER" != "UNKNOWN" ]; then
        echo -e "${GREEN}  Created: ${INC_NUMBER}${NC}"
    else
        echo -e "${RED}  Failed to create incident${NC}"
        echo "  Response: $RESULT"
    fi
}

echo -e "${BLUE}Creating demo incidents...${NC}"
echo ""

# Incident 1: Critical database connection issue
echo -e "${YELLOW}Creating: Database connection timeout on production cluster${NC}"
RESULT=$(curl -s -X POST \
    -H "${AUTH_HEADER}" \
    -H "Content-Type: application/json" \
    -H "Accept: application/json" \
    "${INSTANCE_URL}/api/now/table/incident" \
    --data-raw '{"short_description":"Database connection timeout on production cluster","description":"Production database cluster experiencing intermittent connection timeouts. Error rate spiked to 15% starting at 14:00 UTC. Databricks ETL jobs are failing. Timeline: 13:55 UTC connection pool exhaustion, 13:56 UTC first ETL failures, 14:00 UTC error rate 15%. Affected: production-db-cluster-01, Daily ETL Pipeline (job 101). Max connections reached (500/500).","priority":"1","category":"Database","subcategory":"Performance","urgency":"1","impact":"1","correlation_id":"SUSE-AI-DEMO"}')
INC_NUMBER=$(echo "$RESULT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('result',{}).get('number','FAILED'))" 2>/dev/null || echo "FAILED")
echo -e "${GREEN}  Created: ${INC_NUMBER}${NC}"

# Incident 2: Databricks cluster scaling issue
echo -e "${YELLOW}Creating: Databricks cluster auto-scaling failure${NC}"
RESULT=$(curl -s -X POST \
    -H "${AUTH_HEADER}" \
    -H "Content-Type: application/json" \
    -H "Accept: application/json" \
    "${INSTANCE_URL}/api/now/table/incident" \
    --data-raw '{"short_description":"Databricks cluster auto-scaling failure","description":"Databricks production-etl-cluster failed to auto-scale during peak processing. Cluster stuck at 8 workers (max 16). Job queue depth: 45 pending jobs. Processing latency increased 3x. Cluster: production-etl-cluster (1234-567890-abc123), Spark 13.3.x-scala2.12.","priority":"2","category":"Cloud Infrastructure","subcategory":"Databricks","urgency":"2","impact":"2","correlation_id":"SUSE-AI-DEMO"}')
INC_NUMBER=$(echo "$RESULT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('result',{}).get('number','FAILED'))" 2>/dev/null || echo "FAILED")
echo -e "${GREEN}  Created: ${INC_NUMBER}${NC}"

# Incident 3: Data quality issue
echo -e "${YELLOW}Creating: ETL pipeline data quality alert${NC}"
RESULT=$(curl -s -X POST \
    -H "${AUTH_HEADER}" \
    -H "Content-Type: application/json" \
    -H "Accept: application/json" \
    "${INSTANCE_URL}/api/now/table/incident" \
    --data-raw '{"short_description":"ETL pipeline data quality alert","description":"Data quality checks failing on customer_transactions table. Null values in required fields. Table: main.production.customer_transactions. Failed records: 15000 (1.2%). Affected fields: customer_id, transaction_date. Root cause: upstream system sending incomplete records.","priority":"3","category":"Data Platform","subcategory":"ETL","urgency":"2","impact":"3","correlation_id":"SUSE-AI-DEMO"}')
INC_NUMBER=$(echo "$RESULT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('result',{}).get('number','FAILED'))" 2>/dev/null || echo "FAILED")
echo -e "${GREEN}  Created: ${INC_NUMBER}${NC}"

# Incident 4: ML model performance degradation
echo -e "${YELLOW}Creating: ML model prediction accuracy degradation${NC}"
RESULT=$(curl -s -X POST \
    -H "${AUTH_HEADER}" \
    -H "Content-Type: application/json" \
    -H "Accept: application/json" \
    "${INSTANCE_URL}/api/now/table/incident" \
    --data-raw '{"short_description":"ML model prediction accuracy degradation","description":"Production ML model showing degraded accuracy. F1 score dropped from 0.92 to 0.78 in 24 hours. Model: customer_churn_predictor_v3. Training cluster: ml-training-cluster. Last retrain: 7 days ago. Suspected: data drift, upstream quality issues, possible correlation with DB connectivity issues.","priority":"3","category":"Application","subcategory":"Machine Learning","urgency":"3","impact":"3","correlation_id":"SUSE-AI-DEMO"}')
INC_NUMBER=$(echo "$RESULT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('result',{}).get('number','FAILED'))" 2>/dev/null || echo "FAILED")
echo -e "${GREEN}  Created: ${INC_NUMBER}${NC}"

echo ""
echo -e "${GREEN}╔══════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║         Demo data loaded successfully!                           ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${BLUE}Demo incidents tagged with: correlation_id=${DEMO_CORRELATION_ID}${NC}"
echo ""
echo "To filter demo incidents in Claude Code, ask:"
echo "  'List incidents with correlation_id SUSE-AI-DEMO'"
echo "  'Show me all SUSE-AI-DEMO incidents'"
echo ""
echo "You can view the incidents at:"
echo "  ${INSTANCE_URL}/nav_to.do?uri=incident_list.do"
echo ""
echo "Or query via API:"
echo "  curl -H \"${AUTH_HEADER:0:20}...\" \\"
echo "    \"${INSTANCE_URL}/api/now/table/incident?sysparm_query=correlation_id=${DEMO_CORRELATION_ID}\""
