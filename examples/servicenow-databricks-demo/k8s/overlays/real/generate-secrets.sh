#!/bin/bash
# Generate secrets.yaml from environment variables
#
# Supports multiple ServiceNow authentication methods:
# - OAuth 2.0 (recommended)
# - Personal Access Token (PAT)
# - Basic Auth (legacy)
#
# Usage:
#   # OAuth 2.0 (recommended)
#   export SERVICENOW_INSTANCE_URL="https://devXXXXX.service-now.com"
#   export SERVICENOW_AUTH_METHOD="oauth"
#   export SERVICENOW_CLIENT_ID="your_client_id"
#   export SERVICENOW_CLIENT_SECRET="your_client_secret"
#
#   # Personal Access Token
#   export SERVICENOW_INSTANCE_URL="https://devXXXXX.service-now.com"
#   export SERVICENOW_AUTH_METHOD="pat"
#   export SERVICENOW_PAT="your_personal_access_token"
#
#   # Basic Auth (legacy)
#   export SERVICENOW_INSTANCE_URL="https://devXXXXX.service-now.com"
#   export SERVICENOW_AUTH_METHOD="basic"
#   export SERVICENOW_USERNAME="admin"
#   export SERVICENOW_PASSWORD="your_password"
#
#   # Databricks
#   export DATABRICKS_HOST="https://dbc-xxxxx.cloud.databricks.com"
#   export DATABRICKS_TOKEN="dapi-xxxxxxxx"
#
#   ./generate-secrets.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Default auth method
SERVICENOW_AUTH_METHOD="${SERVICENOW_AUTH_METHOD:-oauth}"

# Check required variables
if [ -z "$SERVICENOW_INSTANCE_URL" ]; then
    echo "Error: SERVICENOW_INSTANCE_URL not set"
    exit 1
fi

# Validate auth method specific credentials
case "$SERVICENOW_AUTH_METHOD" in
    oauth)
        if [ -z "$SERVICENOW_CLIENT_ID" ] || [ -z "$SERVICENOW_CLIENT_SECRET" ]; then
            echo "Error: OAuth requires SERVICENOW_CLIENT_ID and SERVICENOW_CLIENT_SECRET"
            exit 1
        fi
        ;;
    pat)
        if [ -z "$SERVICENOW_PAT" ]; then
            echo "Error: PAT auth requires SERVICENOW_PAT"
            exit 1
        fi
        ;;
    basic)
        if [ -z "$SERVICENOW_USERNAME" ] || [ -z "$SERVICENOW_PASSWORD" ]; then
            echo "Error: Basic auth requires SERVICENOW_USERNAME and SERVICENOW_PASSWORD"
            exit 1
        fi
        ;;
    *)
        echo "Error: Invalid auth method '$SERVICENOW_AUTH_METHOD'. Use 'oauth', 'pat', or 'basic'"
        exit 1
        ;;
esac

if [ -z "$DATABRICKS_HOST" ] || [ -z "$DATABRICKS_TOKEN" ]; then
    echo "Error: Databricks credentials not set"
    echo "Required: DATABRICKS_HOST, DATABRICKS_TOKEN"
    exit 1
fi

# Helper function to base64 encode, handling empty values
b64_encode() {
    if [ -n "$1" ]; then
        echo -n "$1" | base64
    else
        echo ""
    fi
}

# Generate secrets.yaml
cat > "$SCRIPT_DIR/secrets.yaml" << EOF
# Auto-generated secrets file
# Generated on: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
# Auth method: ${SERVICENOW_AUTH_METHOD}
apiVersion: v1
kind: Secret
metadata:
  name: servicenow-credentials
  namespace: servicenow-databricks-demo
type: Opaque
data:
  SERVICENOW_INSTANCE_URL: $(b64_encode "$SERVICENOW_INSTANCE_URL")
  SERVICENOW_AUTH_METHOD: $(b64_encode "$SERVICENOW_AUTH_METHOD")
  # OAuth 2.0
  SERVICENOW_CLIENT_ID: $(b64_encode "$SERVICENOW_CLIENT_ID")
  SERVICENOW_CLIENT_SECRET: $(b64_encode "$SERVICENOW_CLIENT_SECRET")
  # PAT
  SERVICENOW_PAT: $(b64_encode "$SERVICENOW_PAT")
  # Basic Auth
  SERVICENOW_USERNAME: $(b64_encode "$SERVICENOW_USERNAME")
  SERVICENOW_PASSWORD: $(b64_encode "$SERVICENOW_PASSWORD")
---
apiVersion: v1
kind: Secret
metadata:
  name: databricks-credentials
  namespace: servicenow-databricks-demo
type: Opaque
data:
  DATABRICKS_HOST: $(b64_encode "$DATABRICKS_HOST")
  DATABRICKS_TOKEN: $(b64_encode "$DATABRICKS_TOKEN")
EOF

echo "Generated $SCRIPT_DIR/secrets.yaml"
echo "ServiceNow auth method: $SERVICENOW_AUTH_METHOD"
echo "Deploy with: kubectl apply -k $SCRIPT_DIR"
