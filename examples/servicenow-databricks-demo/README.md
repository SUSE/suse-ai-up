# ServiceNow + Databricks Incident Correlation Demo

This demo showcases how the **SUSE AI Universal Proxy** enables AI assistants to work with enterprise systems through a unified MCP endpoint.

## Demo Scenario

An AI assistant investigates a production incident by:

1. **Discovering** high-priority incidents in ServiceNow
2. **Correlating** with error metrics from Databricks
3. **Identifying** the root cause (database connection pool exhaustion)
4. **Mapping** affected configuration items
5. **Updating** the incident with findings

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      AI Assistant (Claude Code)                  │
└───────────────────────────┬─────────────────────────────────────┘
                            │ MCP Protocol (single connection)
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                  SUSE AI Universal Proxy                         │
│                                                                  │
│  Unified Endpoint: /api/v1/mcp                                  │
│  - Aggregates tools from all adapters                           │
│  - Tool naming: adapter__toolname                               │
│  - Routes calls to correct backend                              │
│                                                                  │
│  Adapters:                                                       │
│  ├── servicenow (17 tools)                                      │
│  └── databricks (10 tools)                                      │
└───────────┬───────────────────────────────────┬─────────────────┘
            │                                   │
            ▼                                   ▼
┌───────────────────────┐           ┌───────────────────────┐
│   ServiceNow MCP      │           │   Databricks MCP      │
│   (Pod: port 8000)    │           │   (Pod: port 8001)    │
└───────────┬───────────┘           └───────────┬───────────┘
            │                                   │
            ▼                                   ▼
    ServiceNow API                      Databricks API
    (Real Instance)                     (Real Workspace)
```

## Prerequisites

- **Rancher Desktop** with Kubernetes enabled
- **kubectl** configured to connect to Rancher Desktop
- **curl** and **jq** for testing

## Quick Start

### 1. Deploy to Kubernetes

```bash
cd examples/servicenow-databricks-demo

# Make scripts executable
chmod +x scripts/*.sh

# Deploy to Rancher Desktop's k3s cluster
kubectl apply -k k8s/overlays/production/
```

### 2. Start Port Forward

```bash
kubectl port-forward -n servicenow-databricks-demo svc/suse-ai-proxy 8911:8911 &
```

### 3. Register Adapters

```bash
./scripts/register-adapters.sh
```

### 4. Load Demo Data

```bash
./scripts/load-servicenow-data.sh --reset
```

### 5. Connect Claude Code

```bash
# Add the unified MCP endpoint
claude mcp add suse-ai-proxy http://localhost:8911/api/v1/mcp \
  --transport http \
  -H "X-User-ID: admin"

# Verify connection
claude
# Then type /mcp to see connected tools
```

### 6. Try Demo Queries

In Claude Code:
```
List incidents with correlation_id SUSE-AI-DEMO
```

```
Get details for incident INC0010014
```

```
Check Databricks error metrics for the last 6 hours
```

## Tool Naming Convention

Tools are prefixed with the adapter name:
- `servicenow__search_incidents`
- `servicenow__get_incident`
- `servicenow__update_incident`
- `databricks__get_error_metrics`
- `databricks__execute_sql`

## Demo Data

Demo incidents are tagged with `correlation_id=SUSE-AI-DEMO` for easy filtering.

To reset demo data:
```bash
./scripts/load-servicenow-data.sh --reset
```

## Real Mode Setup

### ServiceNow Configuration

Create a `.env` file:
```bash
SERVICENOW_MOCK_MODE=false
SERVICENOW_INSTANCE_URL=https://devXXXXX.service-now.com
SERVICENOW_AUTH_METHOD=basic
SERVICENOW_USERNAME=your_username
SERVICENOW_PASSWORD=your_password
```

### Databricks Configuration

Add to `.env`:
```bash
DATABRICKS_MOCK_MODE=false
DATABRICKS_HOST=https://dbc-xxxxx.cloud.databricks.com
DATABRICKS_TOKEN=dapi-xxxxxxxx
DATABRICKS_WAREHOUSE_ID=your_warehouse_id
```

Then redeploy:
```bash
kubectl apply -k k8s/overlays/production/
```

## Directory Structure

```
servicenow-databricks-demo/
├── README.md                     # This file
├── DEMO_SCRIPT.md               # Customer demo walkthrough
├── PRODUCTION_SETUP.md          # Production deployment guide
├── docker-compose.yaml          # Alternative: Docker Compose deployment
├── claude-code-mcp-config.json  # Example Claude Code MCP config
├── servers/
│   ├── servicenow_mcp_server.py # ServiceNow MCP server
│   ├── databricks_mcp_server.py # Databricks MCP server
│   ├── Dockerfile.servicenow
│   └── Dockerfile.databricks
├── k8s/
│   ├── kustomization.yaml       # Base kustomization
│   ├── namespace.yaml
│   ├── configmap.yaml
│   ├── proxy-deployment.yaml
│   ├── servicenow-deployment.yaml
│   ├── databricks-deployment.yaml
│   └── overlays/
│       ├── mock/                # Mock mode overlay
│       ├── real/                # Real mode with secrets
│       └── production/          # Production overlay
└── scripts/
    ├── register-adapters.sh     # Register MCP servers with proxy
    ├── load-servicenow-data.sh  # Load/reset demo incidents
    ├── load-databricks-data.sh  # Load demo analytics data
    ├── setup-rancher-desktop.sh # Initial setup
    └── run-demo.sh              # Scripted demo
```

## MCP Tools Reference

### ServiceNow Tools

| Tool | Description |
|------|-------------|
| `servicenow__search_incidents` | Search by priority, state, category, correlation_id |
| `servicenow__get_incident` | Get incident details by number |
| `servicenow__create_incident` | Create a new incident |
| `servicenow__update_incident` | Update incident (state, work notes) |
| `servicenow__get_related_cis` | Get related configuration items |
| `servicenow__search_cmdb` | Search the CMDB |
| `servicenow__get_incident_metrics` | Get aggregate metrics |

### Databricks Tools

| Tool | Description |
|------|-------------|
| `databricks__list_clusters` | List clusters and status |
| `databricks__get_cluster` | Get cluster details |
| `databricks__list_jobs` | List Databricks jobs |
| `databricks__get_job_runs` | Get job run history |
| `databricks__execute_sql` | Execute SQL queries |
| `databricks__get_error_metrics` | Get error rates and metrics |
| `databricks__get_system_health` | Get system health dashboard |
| `databricks__correlate_events` | Correlate events across systems |

## Testing via curl

### Through Unified Endpoint (Recommended)

```bash
# List all tools
curl -X POST http://localhost:8911/api/v1/mcp \
  -H "Content-Type: application/json" \
  -H "X-User-ID: admin" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": {}}'

# Search demo incidents
curl -X POST http://localhost:8911/api/v1/mcp \
  -H "Content-Type: application/json" \
  -H "X-User-ID: admin" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {
    "name": "servicenow__search_incidents",
    "arguments": {"correlation_id": "SUSE-AI-DEMO"}
  }}'

# Get Databricks error metrics
curl -X POST http://localhost:8911/api/v1/mcp \
  -H "Content-Type: application/json" \
  -H "X-User-ID: admin" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {
    "name": "databricks__get_error_metrics",
    "arguments": {"time_range": "last_6_hours"}
  }}'
```

## Alternative: Docker Compose

For running without Kubernetes:

```bash
# Build images
docker build -t suse-ai-up:local ../../
docker build -t servicenow-mcp:local -f servers/Dockerfile.servicenow servers/
docker build -t databricks-mcp:local -f servers/Dockerfile.databricks servers/

# Start services
docker-compose up -d

# Register adapters (use --local flag for localhost endpoints)
./scripts/register-adapters.sh --local
```

## Viewing Logs

```bash
# Proxy logs (shows tool calls with timing)
kubectl logs -n servicenow-databricks-demo deployment/suse-ai-proxy -f

# Filter for tool calls
kubectl logs -n servicenow-databricks-demo deployment/suse-ai-proxy | grep TOOL_CALL

# MCP server logs
kubectl logs -n servicenow-databricks-demo deployment/servicenow-mcp
kubectl logs -n servicenow-databricks-demo deployment/databricks-mcp
```

## Troubleshooting

### Port forward died
```bash
lsof -ti:8911 | xargs kill -9 2>/dev/null
kubectl port-forward -n servicenow-databricks-demo svc/suse-ai-proxy 8911:8911 &
```

### Adapters not registered
```bash
./scripts/register-adapters.sh
```

### Claude Code shows "Capabilities: none"
Ensure you added the X-User-ID header:
```bash
claude mcp remove suse-ai-proxy -s local
claude mcp add suse-ai-proxy http://localhost:8911/api/v1/mcp \
  --transport http \
  -H "X-User-ID: admin"
```

### Check pod status
```bash
kubectl get pods -n servicenow-databricks-demo
kubectl describe pod -n servicenow-databricks-demo <pod-name>
```

## Cleanup

```bash
kubectl delete namespace servicenow-databricks-demo
```
