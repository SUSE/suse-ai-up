#!/bin/bash
#
# Setup script for ServiceNow + Databricks Demo on Rancher Desktop
#
# This script:
# 1. Builds the MCP server container images
# 2. Loads them into Rancher Desktop's k3s cluster
# 3. Deploys the demo in mock or real mode
#
# Usage:
#   # Mock mode (demo data)
#   ./setup-rancher-desktop.sh
#
#   # Real mode with OAuth 2.0 (recommended)
#   ./setup-rancher-desktop.sh --real \
#     --servicenow-url "https://devXXXXX.service-now.com" \
#     --servicenow-auth oauth \
#     --servicenow-client-id "your_client_id" \
#     --servicenow-client-secret "your_client_secret" \
#     --databricks-host "https://dbc-xxxxx.cloud.databricks.com" \
#     --databricks-token "dapi-xxxxx"
#
#   # Real mode with Personal Access Token
#   ./setup-rancher-desktop.sh --real \
#     --servicenow-url "https://devXXXXX.service-now.com" \
#     --servicenow-auth pat \
#     --servicenow-pat "your_personal_access_token" \
#     --databricks-host "https://dbc-xxxxx.cloud.databricks.com" \
#     --databricks-token "dapi-xxxxx"
#
#   # Real mode with Basic Auth (legacy)
#   ./setup-rancher-desktop.sh --real \
#     --servicenow-url "https://devXXXXX.service-now.com" \
#     --servicenow-auth basic \
#     --servicenow-user "admin" \
#     --servicenow-pass "password" \
#     --databricks-host "https://dbc-xxxxx.cloud.databricks.com" \
#     --databricks-token "dapi-xxxxx"

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEMO_DIR="$(dirname "$SCRIPT_DIR")"
K8S_DIR="$DEMO_DIR/k8s"
SERVERS_DIR="$DEMO_DIR/servers"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
MODE="mock"
SERVICENOW_INSTANCE_URL=""
SERVICENOW_AUTH_METHOD="oauth"
SERVICENOW_CLIENT_ID=""
SERVICENOW_CLIENT_SECRET=""
SERVICENOW_PAT=""
SERVICENOW_USERNAME=""
SERVICENOW_PASSWORD=""
DATABRICKS_HOST=""
DATABRICKS_TOKEN=""

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --real)
            MODE="real"
            shift
            ;;
        --servicenow-url)
            SERVICENOW_INSTANCE_URL="$2"
            shift 2
            ;;
        --servicenow-auth)
            SERVICENOW_AUTH_METHOD="$2"
            shift 2
            ;;
        --servicenow-client-id)
            SERVICENOW_CLIENT_ID="$2"
            shift 2
            ;;
        --servicenow-client-secret)
            SERVICENOW_CLIENT_SECRET="$2"
            shift 2
            ;;
        --servicenow-pat)
            SERVICENOW_PAT="$2"
            shift 2
            ;;
        --servicenow-user)
            SERVICENOW_USERNAME="$2"
            shift 2
            ;;
        --servicenow-pass)
            SERVICENOW_PASSWORD="$2"
            shift 2
            ;;
        --databricks-host)
            DATABRICKS_HOST="$2"
            shift 2
            ;;
        --databricks-token)
            DATABRICKS_TOKEN="$2"
            shift 2
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --real                         Use real mode (connect to actual APIs)"
            echo ""
            echo "ServiceNow Options:"
            echo "  --servicenow-url URL           ServiceNow instance URL"
            echo "  --servicenow-auth METHOD       Auth method: oauth (default), pat, or basic"
            echo ""
            echo "  OAuth 2.0 (recommended):"
            echo "  --servicenow-client-id ID      OAuth client ID"
            echo "  --servicenow-client-secret SEC OAuth client secret"
            echo ""
            echo "  Personal Access Token:"
            echo "  --servicenow-pat TOKEN         Personal access token"
            echo ""
            echo "  Basic Auth (legacy):"
            echo "  --servicenow-user USER         Username"
            echo "  --servicenow-pass PASS         Password"
            echo ""
            echo "Databricks Options:"
            echo "  --databricks-host HOST         Databricks workspace URL"
            echo "  --databricks-token TOKEN       Personal access token"
            echo ""
            echo "  --help, -h                     Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

echo -e "${BLUE}"
echo "╔══════════════════════════════════════════════════════════════════╗"
echo "║     ServiceNow + Databricks Demo Setup for Rancher Desktop       ║"
echo "╚══════════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

# Check prerequisites
echo -e "${YELLOW}Checking prerequisites...${NC}"

if ! command -v kubectl &> /dev/null; then
    echo -e "${RED}Error: kubectl not found. Please install kubectl.${NC}"
    exit 1
fi

if ! command -v nerdctl &> /dev/null && ! command -v docker &> /dev/null; then
    echo -e "${RED}Error: Neither nerdctl nor docker found.${NC}"
    echo "Please ensure Rancher Desktop is running with container runtime enabled."
    exit 1
fi

# Determine container runtime
if command -v nerdctl &> /dev/null; then
    CONTAINER_CMD="nerdctl"
    # Check if we need to use --namespace for Rancher Desktop
    if nerdctl --namespace k8s.io images &> /dev/null; then
        CONTAINER_CMD="nerdctl --namespace k8s.io"
    fi
else
    CONTAINER_CMD="docker"
fi

echo -e "${GREEN}Using container runtime: $CONTAINER_CMD${NC}"

# Check Kubernetes connectivity
if ! kubectl cluster-info &> /dev/null; then
    echo -e "${RED}Error: Cannot connect to Kubernetes cluster.${NC}"
    echo "Please ensure Rancher Desktop is running and k3s is enabled."
    exit 1
fi

echo -e "${GREEN}Connected to Kubernetes cluster${NC}"

# Build container images
echo ""
echo -e "${YELLOW}Building MCP server images...${NC}"

echo "Building ServiceNow MCP server..."
$CONTAINER_CMD build -t servicenow-mcp:local -f "$SERVERS_DIR/Dockerfile.servicenow" "$SERVERS_DIR"

echo "Building Databricks MCP server..."
$CONTAINER_CMD build -t databricks-mcp:local -f "$SERVERS_DIR/Dockerfile.databricks" "$SERVERS_DIR"

echo -e "${GREEN}Images built successfully${NC}"

# Deploy to Kubernetes
echo ""
echo -e "${YELLOW}Deploying to Kubernetes (mode: $MODE)...${NC}"

if [ "$MODE" == "real" ]; then
    # Validate ServiceNow credentials based on auth method
    if [ -z "$SERVICENOW_INSTANCE_URL" ]; then
        echo -e "${RED}Error: ServiceNow instance URL required for real mode${NC}"
        echo "Please provide --servicenow-url"
        exit 1
    fi

    case "$SERVICENOW_AUTH_METHOD" in
        oauth)
            if [ -z "$SERVICENOW_CLIENT_ID" ] || [ -z "$SERVICENOW_CLIENT_SECRET" ]; then
                echo -e "${RED}Error: OAuth requires client ID and secret${NC}"
                echo "Please provide --servicenow-client-id and --servicenow-client-secret"
                exit 1
            fi
            echo -e "${GREEN}ServiceNow auth: OAuth 2.0${NC}"
            ;;
        pat)
            if [ -z "$SERVICENOW_PAT" ]; then
                echo -e "${RED}Error: PAT auth requires personal access token${NC}"
                echo "Please provide --servicenow-pat"
                exit 1
            fi
            echo -e "${GREEN}ServiceNow auth: Personal Access Token${NC}"
            ;;
        basic)
            if [ -z "$SERVICENOW_USERNAME" ] || [ -z "$SERVICENOW_PASSWORD" ]; then
                echo -e "${RED}Error: Basic auth requires username and password${NC}"
                echo "Please provide --servicenow-user and --servicenow-pass"
                exit 1
            fi
            echo -e "${YELLOW}ServiceNow auth: Basic (legacy - consider OAuth or PAT)${NC}"
            ;;
        *)
            echo -e "${RED}Error: Invalid auth method '$SERVICENOW_AUTH_METHOD'${NC}"
            echo "Use: oauth, pat, or basic"
            exit 1
            ;;
    esac

    if [ -z "$DATABRICKS_HOST" ] || [ -z "$DATABRICKS_TOKEN" ]; then
        echo -e "${RED}Error: Databricks credentials required for real mode${NC}"
        echo "Please provide --databricks-host and --databricks-token"
        exit 1
    fi

    # Generate secrets
    echo "Generating secrets..."
    export SERVICENOW_INSTANCE_URL SERVICENOW_AUTH_METHOD
    export SERVICENOW_CLIENT_ID SERVICENOW_CLIENT_SECRET
    export SERVICENOW_PAT SERVICENOW_USERNAME SERVICENOW_PASSWORD
    export DATABRICKS_HOST DATABRICKS_TOKEN
    bash "$K8S_DIR/overlays/real/generate-secrets.sh"

    # Deploy with real overlay
    kubectl apply -k "$K8S_DIR/overlays/real/"
else
    # Deploy with mock overlay
    kubectl apply -k "$K8S_DIR/overlays/mock/"
fi

# Wait for deployments
echo ""
echo -e "${YELLOW}Waiting for deployments to be ready...${NC}"

kubectl wait --for=condition=available --timeout=120s deployment/servicenow-mcp -n servicenow-databricks-demo
kubectl wait --for=condition=available --timeout=120s deployment/databricks-mcp -n servicenow-databricks-demo

echo -e "${GREEN}Deployments ready!${NC}"

# Get service information
echo ""
echo -e "${BLUE}╔══════════════════════════════════════════════════════════════════╗"
echo "║                        Demo Deployed!                             ║"
echo "╚══════════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${GREEN}Mode: $MODE${NC}"
if [ "$MODE" == "real" ]; then
    echo -e "${GREEN}ServiceNow Auth: $SERVICENOW_AUTH_METHOD${NC}"
fi
echo ""
echo "Services:"
kubectl get svc -n servicenow-databricks-demo
echo ""

# Port forwarding instructions
echo -e "${YELLOW}To access the MCP servers locally, run:${NC}"
echo ""
echo "  # ServiceNow MCP (port 8000)"
echo "  kubectl port-forward svc/servicenow-mcp 8000:8000 -n servicenow-databricks-demo &"
echo ""
echo "  # Databricks MCP (port 8001)"
echo "  kubectl port-forward svc/databricks-mcp 8001:8001 -n servicenow-databricks-demo &"
echo ""
echo -e "${YELLOW}Then test with:${NC}"
echo "  curl http://localhost:8000/health"
echo "  curl http://localhost:8001/health"
echo ""
echo -e "${YELLOW}Or run the demo script:${NC}"
echo "  ./scripts/run-demo.sh"
