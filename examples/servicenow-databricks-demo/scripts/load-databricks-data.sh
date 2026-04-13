#!/bin/bash
#
# Load demo analytics data into Databricks
#
# This creates tables with error metrics and system events
# that correlate with ServiceNow incidents for the demo.
#
# Prerequisites:
#   - Databricks workspace with SQL Warehouse
#   - Personal Access Token
#   - Unity Catalog enabled (or adjust catalog/schema names)
#
# Usage:
#   export DATABRICKS_HOST="https://dbc-xxxxx.cloud.databricks.com"
#   export DATABRICKS_TOKEN="dapi-xxxxxxxx"
#   export DATABRICKS_WAREHOUSE_ID="your-warehouse-id"  # Optional
#   ./load-databricks-data.sh

set -e

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
DATABRICKS_HOST="${DATABRICKS_HOST:-}"
DATABRICKS_TOKEN="${DATABRICKS_TOKEN:-}"
WAREHOUSE_ID="${DATABRICKS_WAREHOUSE_ID:-}"
CATALOG="${DATABRICKS_CATALOG:-main}"
SCHEMA="${DATABRICKS_SCHEMA:-analytics}"

echo -e "${BLUE}"
echo "╔══════════════════════════════════════════════════════════════════╗"
echo "║         Loading Demo Data into Databricks                        ║"
echo "╚══════════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

# Validate configuration
if [ -z "$DATABRICKS_HOST" ]; then
    echo -e "${RED}Error: DATABRICKS_HOST is required${NC}"
    echo ""
    echo "Usage:"
    echo "  export DATABRICKS_HOST=\"https://dbc-xxxxx.cloud.databricks.com\""
    echo "  export DATABRICKS_TOKEN=\"dapi-xxxxxxxx\""
    echo "  ./load-databricks-data.sh"
    exit 1
fi

if [ -z "$DATABRICKS_TOKEN" ]; then
    echo -e "${RED}Error: DATABRICKS_TOKEN is required${NC}"
    exit 1
fi

# Remove trailing slash
DATABRICKS_HOST="${DATABRICKS_HOST%/}"

echo -e "${YELLOW}Host: ${DATABRICKS_HOST}${NC}"
echo -e "${YELLOW}Catalog: ${CATALOG}${NC}"
echo -e "${YELLOW}Schema: ${SCHEMA}${NC}"
echo ""

# Test connection
echo -e "${YELLOW}Testing connection...${NC}"
RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" \
    -H "Authorization: Bearer ${DATABRICKS_TOKEN}" \
    "${DATABRICKS_HOST}/api/2.0/clusters/list")

if [ "$RESPONSE" != "200" ]; then
    echo -e "${RED}Failed to connect to Databricks (HTTP ${RESPONSE})${NC}"
    echo "Check your credentials and host URL"
    exit 1
fi
echo -e "${GREEN}Connection successful${NC}"
echo ""

# Find a SQL warehouse if not specified
if [ -z "$WAREHOUSE_ID" ]; then
    echo -e "${YELLOW}Finding SQL warehouse...${NC}"
    WAREHOUSES=$(curl -s \
        -H "Authorization: Bearer ${DATABRICKS_TOKEN}" \
        "${DATABRICKS_HOST}/api/2.0/sql/warehouses")

    WAREHOUSE_ID=$(echo "$WAREHOUSES" | python3 -c "
import sys, json
data = json.load(sys.stdin)
warehouses = data.get('warehouses', [])
# Prefer running warehouse, otherwise first available
for w in warehouses:
    if w.get('state') == 'RUNNING':
        print(w['id'])
        sys.exit(0)
if warehouses:
    print(warehouses[0]['id'])
" 2>/dev/null || echo "")

    if [ -z "$WAREHOUSE_ID" ]; then
        echo -e "${RED}No SQL warehouse found. Please create one or specify DATABRICKS_WAREHOUSE_ID${NC}"
        exit 1
    fi
    echo -e "${GREEN}Found warehouse: ${WAREHOUSE_ID}${NC}"
fi

echo ""

# Function to execute SQL
execute_sql() {
    local statement="$1"
    local description="$2"

    echo -e "${YELLOW}${description}...${NC}"

    RESULT=$(curl -s -X POST \
        -H "Authorization: Bearer ${DATABRICKS_TOKEN}" \
        -H "Content-Type: application/json" \
        "${DATABRICKS_HOST}/api/2.0/sql/statements" \
        -d "{
            \"warehouse_id\": \"${WAREHOUSE_ID}\",
            \"statement\": $(echo "$statement" | python3 -c 'import sys,json; print(json.dumps(sys.stdin.read()))'),
            \"wait_timeout\": \"30s\"
        }")

    STATUS=$(echo "$RESULT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('status',{}).get('state','UNKNOWN'))" 2>/dev/null || echo "ERROR")

    if [ "$STATUS" = "SUCCEEDED" ]; then
        echo -e "${GREEN}  Success${NC}"
    elif [ "$STATUS" = "RUNNING" ] || [ "$STATUS" = "PENDING" ]; then
        echo -e "${YELLOW}  Running (async)${NC}"
    else
        echo -e "${RED}  Failed: ${STATUS}${NC}"
        ERROR=$(echo "$RESULT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('status',{}).get('error',{}).get('message','Unknown error'))" 2>/dev/null || echo "$RESULT")
        echo -e "${RED}  Error: ${ERROR}${NC}"
    fi
}

echo -e "${BLUE}Creating schema and tables...${NC}"
echo ""

# Create schema
execute_sql "CREATE SCHEMA IF NOT EXISTS ${CATALOG}.${SCHEMA}" \
    "Creating schema ${CATALOG}.${SCHEMA}"

# Create error_metrics_hourly table
execute_sql "
CREATE TABLE IF NOT EXISTS ${CATALOG}.${SCHEMA}.error_metrics_hourly (
    timestamp TIMESTAMP,
    hour STRING,
    error_count INT,
    error_rate DOUBLE,
    top_error_type STRING,
    source_system STRING
)
" "Creating error_metrics_hourly table"

# Create system_events table
execute_sql "
CREATE TABLE IF NOT EXISTS ${CATALOG}.${SCHEMA}.system_events (
    event_id STRING,
    timestamp TIMESTAMP,
    event_type STRING,
    source STRING,
    details STRING,
    severity STRING,
    correlation_id STRING
)
" "Creating system_events table"

# Create system_health table
execute_sql "
CREATE TABLE IF NOT EXISTS ${CATALOG}.${SCHEMA}.system_health (
    timestamp TIMESTAMP,
    metric_name STRING,
    current_value DOUBLE,
    threshold DOUBLE,
    status STRING,
    component STRING
)
" "Creating system_health table"

echo ""
echo -e "${BLUE}Loading demo data...${NC}"
echo ""

# Get current timestamp for data
CURRENT_TS=$(date -u +"%Y-%m-%dT%H:%M:%S")
HOUR_AGO=$(date -u -v-1H +"%Y-%m-%dT%H:%M:%S" 2>/dev/null || date -u -d "1 hour ago" +"%Y-%m-%dT%H:%M:%S")

# Load error metrics (showing spike pattern)
execute_sql "
INSERT INTO ${CATALOG}.${SCHEMA}.error_metrics_hourly VALUES
    (current_timestamp() - INTERVAL 6 HOURS, DATE_FORMAT(current_timestamp() - INTERVAL 6 HOURS, 'yyyy-MM-dd HH:00'), 45, 0.05, 'ConnectionTimeout', 'database'),
    (current_timestamp() - INTERVAL 5 HOURS, DATE_FORMAT(current_timestamp() - INTERVAL 5 HOURS, 'yyyy-MM-dd HH:00'), 120, 0.12, 'ConnectionTimeout', 'database'),
    (current_timestamp() - INTERVAL 4 HOURS, DATE_FORMAT(current_timestamp() - INTERVAL 4 HOURS, 'yyyy-MM-dd HH:00'), 230, 0.23, 'ConnectionTimeout', 'database'),
    (current_timestamp() - INTERVAL 3 HOURS, DATE_FORMAT(current_timestamp() - INTERVAL 3 HOURS, 'yyyy-MM-dd HH:00'), 180, 0.18, 'ConnectionTimeout', 'database'),
    (current_timestamp() - INTERVAL 2 HOURS, DATE_FORMAT(current_timestamp() - INTERVAL 2 HOURS, 'yyyy-MM-dd HH:00'), 350, 0.35, 'ConnectionTimeout', 'database'),
    (current_timestamp() - INTERVAL 1 HOURS, DATE_FORMAT(current_timestamp() - INTERVAL 1 HOURS, 'yyyy-MM-dd HH:00'), 280, 0.28, 'ConnectionTimeout', 'database')
" "Loading error metrics data"

# Load system events (incident correlation timeline)
execute_sql "
INSERT INTO ${CATALOG}.${SCHEMA}.system_events VALUES
    ('EVT-001', current_timestamp() - INTERVAL 125 MINUTES, 'DB_CONNECTION_POOL_EXHAUSTED', 'production-db-cluster-01', 'Max connections reached: 500/500', 'CRITICAL', 'CORR-001'),
    ('EVT-002', current_timestamp() - INTERVAL 124 MINUTES, 'ETL_JOB_FAILED', 'Daily ETL Pipeline', 'ConnectionTimeout after 30s - job_id: 101', 'HIGH', 'CORR-001'),
    ('EVT-003', current_timestamp() - INTERVAL 123 MINUTES, 'CLUSTER_SCALE_BLOCKED', 'production-etl-cluster', 'Auto-scale blocked: dependent service unavailable', 'MEDIUM', 'CORR-001'),
    ('EVT-004', current_timestamp() - INTERVAL 120 MINUTES, 'ALERT_TRIGGERED', 'PagerDuty', 'P1 Alert: Database connectivity issue', 'HIGH', 'CORR-001'),
    ('EVT-005', current_timestamp() - INTERVAL 115 MINUTES, 'INCIDENT_CREATED', 'ServiceNow', 'INC0010001: Database connection timeout on production cluster', 'HIGH', 'CORR-001'),
    ('EVT-006', current_timestamp() - INTERVAL 90 MINUTES, 'DATA_QUALITY_ALERT', 'customer_transactions', 'Null values in required fields: 15000 records', 'MEDIUM', 'CORR-002'),
    ('EVT-007', current_timestamp() - INTERVAL 85 MINUTES, 'INCIDENT_CREATED', 'ServiceNow', 'INC0010003: ETL pipeline data quality alert', 'MEDIUM', 'CORR-002')
" "Loading system events data"

# Load system health metrics
execute_sql "
INSERT INTO ${CATALOG}.${SCHEMA}.system_health VALUES
    (current_timestamp(), 'db_connection_pool_used', 95, 80, 'CRITICAL', 'database'),
    (current_timestamp(), 'db_query_latency_p99_ms', 2500, 1000, 'WARNING', 'database'),
    (current_timestamp(), 'db_active_connections', 450, 500, 'WARNING', 'database'),
    (current_timestamp(), 'etl_records_processed', 1250000, 0, 'OK', 'etl'),
    (current_timestamp(), 'etl_failed_records', 15000, 1000, 'CRITICAL', 'etl'),
    (current_timestamp(), 'cluster_worker_count', 8, 4, 'OK', 'compute'),
    (current_timestamp(), 'cluster_memory_utilization', 78, 90, 'OK', 'compute'),
    (current_timestamp(), 'job_queue_depth', 12, 50, 'OK', 'scheduler')
" "Loading system health data"

echo ""
echo -e "${GREEN}╔══════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║         Demo data loaded successfully!                           ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo "Tables created in ${CATALOG}.${SCHEMA}:"
echo "  - error_metrics_hourly  (error rate trends)"
echo "  - system_events         (event timeline for correlation)"
echo "  - system_health         (current health metrics)"
echo ""
echo "You can query the data in Databricks SQL:"
echo ""
echo "  SELECT * FROM ${CATALOG}.${SCHEMA}.error_metrics_hourly ORDER BY timestamp;"
echo "  SELECT * FROM ${CATALOG}.${SCHEMA}.system_events ORDER BY timestamp;"
echo "  SELECT * FROM ${CATALOG}.${SCHEMA}.system_health;"
echo ""
echo "Or view in the Databricks UI:"
echo "  ${DATABRICKS_HOST}/sql/editor"
