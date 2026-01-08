# SUSE AI Universal Proxy - Examples

This document provides comprehensive examples of how to deploy and test various MCP (Model Context Protocol) servers using the SUSE AI Universal Proxy.

## Architecture Overview

The SUSE AI Universal Proxy provides:

- **MCP Registry**: Centralized registry of MCP servers with metadata and configuration
- **Dynamic Adapters**: On-demand creation of MCP server adapters with automatic sidecar deployment
- **Kubernetes Integration**: Native Kubernetes deployments with RBAC and service management
- **Load Balancing**: External access via LoadBalancer service
- **Multi-tenancy**: Isolated sidecar deployments in dedicated namespace

### Key Features

- **Automatic Sidecar Deployment**: MCP servers run as Kubernetes deployments with proper resource management
- **Registry-Driven Configuration**: All server configurations stored in ConfigMaps
- **RBAC Security**: Cluster-level permissions for deployment management
- **Health Monitoring**: Built-in health checks and readiness probes
- **Scalability**: Horizontal pod scaling support

## Prerequisites

1. **Kubernetes Cluster**: Access to a Kubernetes cluster with kubectl configured
2. **Helm 3**: Install Helm 3 for chart deployment
3. **mcpinspector**: Install mcpinspector for testing MCP connections:
   ```bash
   npm install -g @modelcontextprotocol/inspector
   ```

### Helm Chart Installation

Deploy the SUSE AI Universal Proxy using the provided Helm chart:

```bash
# Create required namespaces
kubectl create namespace suse-ai-up
# Note: suse-ai-up-mcp namespace is created automatically by the Helm chart

# Install with LoadBalancer service (recommended for external access)
helm install suse-ai-up ./charts/suse-ai-up/suse-ai-up-1.0.0.tgz \
  --namespace suse-ai-up \
  --set service.type=LoadBalancer \
  --set rbac.create=true \
  --set sidecar.enabled=true

# Wait for deployment
kubectl wait --for=condition=available --timeout=300s deployment/suse-ai-up -n suse-ai-up

# Get service endpoint
SERVICE_IP=$(kubectl get svc suse-ai-up-service -n suse-ai-up -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
SERVICE_PORT=8911
echo "MCP Proxy available at: http://${SERVICE_IP}:${SERVICE_PORT}"
```

### Alternative Installation Methods

#### Install in Default Namespace:
```bash
helm install suse-ai-up ./charts/suse-ai-up/suse-ai-up-1.0.0.tgz \
  --set service.type=LoadBalancer
# Note: suse-ai-up-mcp namespace is created automatically by the Helm chart
```

#### Install in Existing Namespace:
```bash
helm install suse-ai-up ./charts/suse-ai-up/suse-ai-up-1.0.0.tgz \
  --namespace existing-namespace \
  --set service.type=LoadBalancer
```

#### Custom Configuration:
```bash
helm install suse-ai-up ./charts/suse-ai-up/suse-ai-up-1.0.0.tgz \
  --namespace suse-ai-up \
  --set service.type=LoadBalancer \
  --set sidecar.namespace=my-custom-mcp-namespace \
  --set rbac.create=true
```

## Example MCP Servers

### 1. SUSE Bugzilla MCP

**Description**: Official SUSE MCP server for Bugzilla issue tracking and bug management with automatic sidecar deployment.

**Search for the MCP in Registry**:
```bash
# Get service endpoint
SERVICE_IP=$(kubectl get svc suse-ai-up-service -n suse-ai-up -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
SERVICE_PORT=8911

# Search for Bugzilla in registry
curl -X GET "http://${SERVICE_IP}:${SERVICE_PORT}/api/v1/registry/browse?q=bugzilla" \
  -H "Content-Type: application/json" | jq .
```

**Create Adapter** (Automatic Sidecar Deployment):
```bash
curl -X POST "http://${SERVICE_IP}:8911/api/v1/adapters" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "bugzilla-adapter",
    "mcpServerId": "bugzilla",
    "environmentVariables": {
      "BUGZILLA_SERVER": "https://bugzilla.suse.com",
      "BUGZILLA_APIKEY": "your-api-key-here"
    }
  }' | jq .
```

**Note**: This automatically deploys a Kubernetes sidecar container in the `suse-ai-up-mcp` namespace with the Docker command from the registry configuration.

**Verify Sidecar Deployment**:
```bash
# Check deployment created
kubectl get deployments -n suse-ai-up-mcp

# Check service created
kubectl get services -n suse-ai-up-mcp

# View sidecar logs
kubectl logs -n suse-ai-up-mcp deployment/mcp-sidecar-bugzilla-adapter
```

**Connect using mcpinspector**:
```bash
mcpinspector "http://${SERVICE_IP}:8911/api/v1/adapters/bugzilla-adapter/mcp"
```

### 2. SUSE Uyuni MCP

**Description**: Official SUSE MCP server for Uyuni server management, patch deployment, and system administration with automatic sidecar deployment.

**Environment Variables**:
The following environment variables can be configured when creating the adapter:

```bash
# Required: Basic server parameters
UYUNI_SERVER=your-uyuni-server.example.com:443
UYUNI_USER=admin
UYUNI_PASS=your-admin-password

# Optional: SSL certificate verification (default: true)
UYUNI_MCP_SSL_VERIFY=false

# Optional: Enable write operations (default: false)
UYUNI_MCP_WRITE_TOOLS_ENABLED=false

# Optional: SSH private key for system bootstrapping
UYUNI_SSH_PRIV_KEY="-----BEGIN OPENSSH PRIVATE KEY-----\nyour-private-key-here\n-----END OPENSSH PRIVATE KEY-----"
UYUNI_SSH_PRIV_KEY_PASS="your-key-passphrase"
```

**Search for the MCP in Registry**:
```bash
# Get service endpoint
SERVICE_IP=$(kubectl get svc suse-ai-up-service -n suse-ai-up -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
SERVICE_PORT=8911

# Search for Uyuni in registry
curl -X GET "http://${SERVICE_IP}:${SERVICE_PORT}/api/v1/registry/browse?q=uyuni" \
  -H "Content-Type: application/json" | jq .
```

**Create Adapter** (Automatic Sidecar Deployment):
```bash
curl -X POST "http://${SERVICE_IP}:8911/api/v1/adapters" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "uyuni-adapter",
    "mcpServerId": "uyuni",
    "environmentVariables": {
      "UYUNI_SERVER": "your-uyuni-server.example.com:443",
      "UYUNI_USER": "admin",
      "UYUNI_PASS": "your-admin-password",
      "UYUNI_MCP_SSL_VERIFY": "false",
      "UYUNI_MCP_TRANSPORT": "http"
    }
  }' | jq .
```

**Note**: This automatically deploys a Kubernetes sidecar container running the Uyuni MCP server with the Docker command from the registry configuration.

**Verify Sidecar Deployment**:
```bash
# Check deployment created
kubectl get deployments -n suse-ai-up-mcp

# Check service created
kubectl get services -n suse-ai-up-mcp

# View sidecar logs
kubectl logs -n suse-ai-up-mcp deployment/mcp-sidecar-uyuni-adapter
```

**Connect using mcpinspector**:
```bash
mcpinspector "http://${SERVICE_IP}:8911/api/v1/adapters/uyuni-adapter/mcp"
```

### 3. Browse All Available MCP Servers

**Description**: Explore all MCP servers available in the registry.

**Browse Registry**:
```bash
# Get all available servers
curl -X GET "http://${SERVICE_IP}:${SERVICE_PORT}/api/v1/registry/browse" \
  -H "Content-Type: application/json" | jq .

# Search by category
curl -X GET "http://${SERVICE_IP}:${SERVICE_PORT}/api/v1/registry/browse?q=ai-ml" \
  -H "Content-Type: application/json" | jq .

# Search by tags
curl -X GET "http://${SERVICE_IP}:${SERVICE_PORT}/api/v1/registry/browse?q=kubernetes" \
  -H "Content-Type: application/json" | jq .
```

**Registry Response Example**:
```json
[
  {
    "id": "uyuni",
    "name": "uyuni",
    "description": "",
    "_meta": {
      "category": "system-management",
      "registry_source": "yaml",
      "sidecarConfig": {
        "command": "docker run -it --rm -e UYUNI_SERVER=http://dummy.domain.com...",
        "commandType": "docker",
        "port": 8000
      }
    }
  }
]
```

## Common Operations

### Get Service Endpoints
```bash
# Get LoadBalancer IP
SERVICE_IP=$(kubectl get svc suse-ai-up-service -n suse-ai-up -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
PROXY_PORT=8911
REGISTRY_PORT=8911

echo "Proxy: http://${SERVICE_IP}:${PROXY_PORT}"
echo "Registry: http://${SERVICE_IP}:${REGISTRY_PORT}"
```

### List All Adapters
```bash
curl -X GET "http://${SERVICE_IP}:${PROXY_PORT}/api/v1/adapters" \
  -H "Content-Type: application/json" | jq .
```

### Get Adapter Details
```bash
curl -X GET "http://${SERVICE_IP}:${PROXY_PORT}/api/v1/adapters/{adapter-name}" \
  -H "Content-Type: application/json" | jq .
```

### Delete Adapter
```bash
curl -X DELETE "http://${SERVICE_IP}:${PROXY_PORT}/api/v1/adapters/{adapter-name}" \
  -H "Content-Type: application/json" | jq .
```

**Note**: When deleting adapters with sidecar deployments, the associated Kubernetes deployment and service resources in the `suse-ai-up-mcp` namespace are automatically cleaned up.

### Search Registry
```bash
# Search by name, category, or tags
curl -X GET "http://${SERVICE_IP}:${REGISTRY_PORT}/api/v1/registry/browse?q={search-term}" \
  -H "Content-Type: application/json" | jq .

# Filter by source (yaml for built-in servers)
curl -X GET "http://${SERVICE_IP}:${REGISTRY_PORT}/api/v1/registry/browse?source=yaml" \
  -H "Content-Type: application/json" | jq .
```

## Troubleshooting

### Installation Issues
```bash
# Check Helm release status
helm list -n suse-ai-up

# Check pod status
kubectl get pods -n suse-ai-up

# View deployment logs
kubectl logs -n suse-ai-up deployment/suse-ai-up

# Check service endpoints
kubectl get svc -n suse-ai-up
```

### RBAC Permission Issues
```bash
# Check cluster role exists
kubectl get clusterrole suse-ai-up-role

# Check cluster role binding
kubectl get clusterrolebinding suse-ai-up-rolebinding

# Check service account
kubectl get serviceaccount -n suse-ai-up

# Verify permissions
kubectl auth can-i create deployments --as=system:serviceaccount:suse-ai-up:suse-ai-up-proxy-sa -n suse-ai-up-mcp
```

### Adapter Creation Fails
- Check that the MCP server exists in the registry: `curl "http://${SERVICE_IP}:${REGISTRY_PORT}/api/v1/registry/browse"`
- Verify environment variables are correctly set
- Ensure the `suse-ai-up-mcp` namespace exists: `kubectl get namespace suse-ai-up-mcp`
- Check sidecar deployment permissions in the target namespace
- View proxy service logs: `kubectl logs -n suse-ai-up deployment/suse-ai-up`

### Sidecar Deployment Issues
```bash
# Check sidecar namespace
kubectl get namespace suse-ai-up-mcp

# Check failed deployments
kubectl get deployments -n suse-ai-up-mcp

# View sidecar logs
kubectl logs -n suse-ai-up-mcp deployment/mcp-sidecar-{adapter-name}

# Check events for errors
kubectl get events -n suse-ai-up-mcp --sort-by=.metadata.creationTimestamp
```

### Connection Issues
- Verify the adapter exists: `curl "http://${SERVICE_IP}:${PROXY_PORT}/api/v1/adapters"`
- Check adapter status and endpoint
- Ensure mcpinspector is properly installed: `mcpinspector --version`
- Test MCP endpoint directly: `curl "http://${SERVICE_IP}:${PROXY_PORT}/api/v1/adapters/{adapter-name}/mcp"`

## Advanced Usage

### Custom Environment Variables
```bash
curl -X POST "http://${SERVICE_IP}:${PROXY_PORT}/api/v1/adapters" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "custom-adapter",
    "mcpServerId": "server-id",
    "environmentVariables": {
      "CUSTOM_VAR": "value",
      "ANOTHER_VAR": "another-value"
    }
  }' | jq .
```

### Using Different MCP Servers
Replace `serverId` with any MCP server ID from the registry. All servers configured in the Helm chart's `values.yaml` under `registry.servers` are available.

### Adapter Management
```bash
# List all adapters
curl "http://${SERVICE_IP}:${PROXY_PORT}/api/v1/adapters"

# Get specific adapter details
curl "http://${SERVICE_IP}:${PROXY_PORT}/api/v1/adapters/my-adapter"

# Delete adapter (also removes sidecar deployment)
curl -X DELETE "http://${SERVICE_IP}:${PROXY_PORT}/api/v1/adapters/my-adapter"
```

### Monitoring Deployments
```bash
# Monitor main service
kubectl get pods -n suse-ai-up -w

# Monitor sidecar deployments
kubectl get pods -n suse-ai-up-mcp -w

# Check resource usage
kubectl top pods -n suse-ai-up
kubectl top pods -n suse-ai-up-mcp
```

### Scaling and Updates
```bash
# Scale the proxy deployment
kubectl scale deployment suse-ai-up -n suse-ai-up --replicas=3

# Update with new registry servers
helm upgrade suse-ai-up ./charts/suse-ai-up/suse-ai-up-1.0.0.tgz \
  --set-file registry.servers=my-updated-servers.yaml

# Rolling restart
kubectl rollout restart deployment/suse-ai-up -n suse-ai-up
```

## Cleanup and Uninstallation

### Remove All Adapters
```bash
# List all adapters
curl "http://${SERVICE_IP}:${PROXY_PORT}/api/v1/adapters" | jq -r '.[] | .id'

# Delete each adapter (this removes sidecar deployments)
for adapter in $(curl -s "http://${SERVICE_IP}:${PROXY_PORT}/api/v1/adapters" | jq -r '.[] | .id'); do
  curl -X DELETE "http://${SERVICE_IP}:${PROXY_PORT}/api/v1/adapters/${adapter}"
done
```

### Uninstall Helm Chart
```bash
# Uninstall the release
helm uninstall suse-ai-up -n suse-ai-up

# Remove RBAC resources
kubectl delete clusterrole suse-ai-up-role
kubectl delete clusterrolebinding suse-ai-up-rolebinding

# Remove namespaces (optional - check for remaining resources first)
kubectl delete namespace suse-ai-up
kubectl delete namespace suse-ai-up-mcp
```

### Verify Cleanup
```bash
# Check for remaining resources
kubectl get all -n suse-ai-up
kubectl get all -n suse-ai-up-mcp
kubectl get clusterrole,clusterrolebinding | grep suse-ai-up
```

## Support and Contributing

- **Issues**: Report bugs and request features on GitHub
- **Documentation**: Full API documentation available at `/docs` endpoint
- **Contributing**: See CONTRIBUTING.md for development guidelines

---

**Note**: This documentation reflects the current state of the SUSE AI Universal Proxy. Features and APIs may evolve over time.