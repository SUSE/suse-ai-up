# Production Demo Setup Guide

This guide walks through setting up the ServiceNow + Databricks demo with **live data** and **Claude Code** as the AI client.

## Architecture

```
┌─────────────┐     ┌──────────────────┐     ┌─────────────────┐
│ Claude Code │────▶│ Universal Proxy  │────▶│ ServiceNow      │
│   (MCP)     │     │  localhost:8911  │     │ (your instance) │
└─────────────┘     │                  │     └─────────────────┘
                    │                  │     ┌─────────────────┐
                    │                  │────▶│ Databricks      │
                    └──────────────────┘     │ (your workspace)│
                                             └─────────────────┘
```

## Prerequisites

- ServiceNow developer instance (Zurich or later for PAT support)
- Databricks workspace with SQL Warehouse access
- Claude Code CLI installed
- Universal Proxy running locally or in Kubernetes

---

## Step 1: ServiceNow Setup

### 1.1 Create a Personal Access Token (PAT)

1. Log into your ServiceNow instance
2. Navigate to: **System OAuth > Personal Access Tokens**
3. Click **New**
4. Set:
   - **Name**: `mcp-demo-token`
   - **Expiration**: Set appropriate date
5. Click **Generate Token**
6. **Copy and save the token** (you won't see it again)

### 1.2 Load Demo Incidents

Run this script to create sample incidents in ServiceNow:

```bash
# Set your credentials
export SERVICENOW_INSTANCE_URL="https://devXXXXX.service-now.com"
export SERVICENOW_PAT="your-personal-access-token"

# Run the data loader
./scripts/load-servicenow-data.sh
```

This creates incidents that correlate with Databricks errors:
- INC0010001: Database connection timeout (Critical)
- INC0010002: Databricks cluster scaling failure (High)
- INC0010003: ETL pipeline data quality alert (Medium)

---

## Step 2: Databricks Setup

### 2.1 Get Your Credentials

1. Log into your Databricks workspace
2. Click your profile icon → **Settings**
3. Go to **Developer** → **Access Tokens**
4. Click **Generate New Token**
5. Copy the token (starts with `dapi`)
6. Note your workspace URL (e.g., `https://dbc-xxxxx.cloud.databricks.com`)

### 2.2 Create Analytics Tables

Run this notebook or SQL to create the demo tables:

```bash
# Set your credentials
export DATABRICKS_HOST="https://dbc-xxxxx.cloud.databricks.com"
export DATABRICKS_TOKEN="dapi-xxxxxxxx"

# Run the data loader
./scripts/load-databricks-data.sh
```

This creates:
- `main.analytics.error_metrics_hourly` - Error rate data
- `main.analytics.system_events` - System event log
- `main.analytics.system_health` - Health metrics

---

## Step 3: Configure MCP Servers for Real Mode

### 3.1 Update Kubernetes Secrets

```bash
# Create secrets for ServiceNow
kubectl create secret generic servicenow-credentials \
  --namespace servicenow-databricks-demo \
  --from-literal=SERVICENOW_INSTANCE_URL="https://devXXXXX.service-now.com" \
  --from-literal=SERVICENOW_PAT="your-pat-token" \
  --from-literal=SERVICENOW_MOCK_MODE="false" \
  --from-literal=SERVICENOW_AUTH_METHOD="pat"

# Create secrets for Databricks
kubectl create secret generic databricks-credentials \
  --namespace servicenow-databricks-demo \
  --from-literal=DATABRICKS_HOST="https://dbc-xxxxx.cloud.databricks.com" \
  --from-literal=DATABRICKS_TOKEN="dapi-xxxxxxxx" \
  --from-literal=DATABRICKS_MOCK_MODE="false"
```

### 3.2 Update Deployments to Use Secrets

Apply the production overlay:

```bash
kubectl apply -k k8s/overlays/production/
```

Or update the deployments manually to reference the secrets.

---

## Step 4: Configure Claude Code

### 4.1 Create MCP Configuration

Create or edit `~/.claude/settings.json`:

```json
{
  "mcpServers": {
    "servicenow": {
      "type": "http",
      "url": "http://localhost:8911/api/v1/adapters/servicenow/mcp",
      "headers": {
        "X-User-ID": "demo-user"
      }
    },
    "databricks": {
      "type": "http",
      "url": "http://localhost:8911/api/v1/adapters/databricks/mcp",
      "headers": {
        "X-User-ID": "demo-user"
      }
    }
  }
}
```

### 4.2 Verify Configuration

```bash
# Check Claude Code sees the MCP servers
claude mcp list

# Test a tool call
claude mcp call servicenow search_incidents '{"limit": 3}'
```

---

## Step 5: Run the Demo

### 5.1 Start the Infrastructure

```bash
# Option A: All in Kubernetes
kubectl apply -k k8s/

# Port forward the proxy
kubectl port-forward -n servicenow-databricks-demo svc/suse-ai-proxy 8911:8911

# Register adapters
./scripts/register-adapters.sh
```

### 5.2 Demo Conversation with Claude Code

Start Claude Code and try these prompts:

```
You: "What critical incidents are currently open in ServiceNow?"

You: "For incident INC0010001, can you check Databricks for related
     errors around the same time?"

You: "What's the current system health in Databricks? Are there any
     metrics that might explain the incident?"

You: "Based on the ServiceNow incident and Databricks data, what do
     you think is the root cause?"
```

---

## Environment Variables Reference

### ServiceNow MCP Server

| Variable | Description | Example |
|----------|-------------|---------|
| `SERVICENOW_MOCK_MODE` | Use mock data (true/false) | `false` |
| `SERVICENOW_INSTANCE_URL` | Your instance URL | `https://devXXXXX.service-now.com` |
| `SERVICENOW_AUTH_METHOD` | Auth type: `oauth`, `pat`, `basic` | `pat` |
| `SERVICENOW_PAT` | Personal Access Token | `xxxxxxxx` |
| `SERVICENOW_CLIENT_ID` | OAuth client ID (if using OAuth) | |
| `SERVICENOW_CLIENT_SECRET` | OAuth client secret (if using OAuth) | |

### Databricks MCP Server

| Variable | Description | Example |
|----------|-------------|---------|
| `DATABRICKS_MOCK_MODE` | Use mock data (true/false) | `false` |
| `DATABRICKS_HOST` | Workspace URL | `https://dbc-xxx.cloud.databricks.com` |
| `DATABRICKS_TOKEN` | Personal access token | `dapi-xxxxxxxx` |

---

## Troubleshooting

### Claude Code can't connect to MCP servers

1. Check proxy is running: `curl http://localhost:8911/health`
2. Check adapters are registered: `curl -H "X-User-ID: admin" http://localhost:8911/api/v1/adapters`
3. Test adapter directly:
   ```bash
   curl -X POST http://localhost:8911/api/v1/adapters/servicenow/mcp \
     -H "Content-Type: application/json" \
     -H "X-User-ID: demo-user" \
     -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}'
   ```

### ServiceNow authentication fails

1. Verify PAT hasn't expired
2. Check instance URL doesn't have trailing slash
3. Test directly:
   ```bash
   curl -H "Authorization: Bearer YOUR_PAT" \
     "https://devXXXXX.service-now.com/api/now/table/incident?sysparm_limit=1"
   ```

### Databricks authentication fails

1. Verify token is valid
2. Check workspace URL
3. Test directly:
   ```bash
   curl -H "Authorization: Bearer dapi-xxxxx" \
     "https://dbc-xxxxx.cloud.databricks.com/api/2.0/clusters/list"
   ```
