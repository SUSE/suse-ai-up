# SUSE AI Universal Proxy Helm Chart

This Helm chart deploys the SUSE AI Universal Proxy with proxy support for executing MCP (Model Context Protocol) commands in a Kubernetes cluster.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- (Optional) Prometheus Operator for monitoring

## Installing the Chart

### Development Environment

```bash
# Add the chart repository (if applicable)
helm repo add mcp-gateway https://charts.example.com/
helm repo update

# Install with development values
helm install mcp-gateway ./deployment/helm/mcp-gateway -f values-dev.yaml
```

### Accessing the API Documentation

Once deployed, you can access the interactive API documentation:

- **Swagger UI**: `http://mcp-gateway-dev.local/docs`
- **API JSON**: `http://mcp-gateway-dev.local/swagger/doc.json`

The Swagger UI provides an interactive interface to explore and test all API endpoints.

### Production Environment

```bash
# Install with production values
helm install mcp-gateway ./deployment/helm/mcp-gateway -f values-prod.yaml \
  --set gateway.auth.oauth.clientId="your-client-id" \
  --set gateway.auth.oauth.tenantId="your-tenant-id" \
  --set gateway.storage.cosmos.connectionString="your-connection-string"
```

## Configuration

### Gateway Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `gateway.enabled` | Enable gateway deployment | `true` |
| `gateway.replicaCount` | Number of gateway replicas | `1` |
| `gateway.image.repository` | Gateway image repository | `ghcr.io/suse/suse-ai-up` |
| `gateway.image.tag` | Gateway image tag | `latest` |
| `gateway.service.type` | Gateway service type | `ClusterIP` |
| `gateway.service.port` | Gateway service port | `8000` |
| `gateway.auth.mode` | Authentication mode (`dev` or `oauth`) | `dev` |
| `gateway.storage.type` | Storage type (`inmemory` or `cosmos`) | `inmemory` |
| `gateway.session.type` | Session type (`inmemory` or `redis`) | `inmemory` |

### Proxy Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `proxy.enabled` | Enable proxy deployment | `true` |
| `proxy.replicaCount` | Number of proxy replicas | `1` |
| `proxy.mcp.mode` | MCP mode (`local` or `remote`) | `local` |
| `proxy.mcp.local.command` | Local MCP command | `npx` |
| `proxy.runtime.allowedCommands` | Allowed runtime commands | `["npx", "uvx", "python", "node", "dotnet"]` |

### Monitoring

| Parameter | Description | Default |
|-----------|-------------|---------|
| `monitoring.enabled` | Enable monitoring | `false` |
| `monitoring.serviceMonitor.enabled` | Enable ServiceMonitor | `false` |
| `monitoring.serviceMonitor.interval` | Scrape interval | `30s` |

## Authentication

### Development Mode

In development mode, authentication is disabled and all requests are allowed.

```yaml
gateway:
  auth:
    mode: "dev"
```

### OAuth Mode

For production, configure OAuth authentication:

```yaml
gateway:
  auth:
    mode: "oauth"
    oauth:
      provider: "azure"
      clientId: "your-client-id"
      tenantId: "your-tenant-id"
      jwksUrl: "https://login.microsoftonline.com/your-tenant-id/discovery/v2.0/keys"
      issuer: "https://login.microsoftonline.com/your-tenant-id/v2.0"
      audience: "your-client-id"
```

## Storage

### In-Memory Storage

Default configuration for development:

```yaml
gateway:
  storage:
    type: "inmemory"
```

### Cosmos DB Storage

For production persistence:

```yaml
gateway:
  storage:
    type: "cosmos"
    cosmos:
      accountEndpoint: "https://your-account.documents.azure.com:443/"
      databaseName: "McpGatewayDb"
```

## MCP Proxy

### Local MCP Servers

Execute MCP commands locally within containers:

```yaml
proxy:
  mcp:
    mode: "local"
    local:
      command: "npx"
      args: "-y @azure/mcp@latest server start"
```

### Remote MCP Servers

Connect to external MCP servers:

```yaml
proxy:
  mcp:
    mode: "remote"
    remote:
      url: "https://internal-mcp-server/mcp"
```

## Security

The chart includes several security features:

- **Network Policies**: Restrict pod-to-pod communication
- **RBAC**: Minimal required permissions for Kubernetes API access
- **Security Contexts**: Non-root containers with dropped capabilities
- **Resource Limits**: Prevent resource exhaustion

## API Endpoints

The SUSE AI Universal Proxy provides the following REST API endpoints:

### Adapter Management
- `POST /adapters` - Create a new MCP server adapter
- `GET /adapters` - List all adapters
- `GET /adapters/{name}` - Get adapter details
- `PUT /adapters/{name}` - Update an adapter
- `DELETE /adapters/{name}` - Delete an adapter
- `GET /adapters/{name}/status` - Get adapter deployment status
- `GET /adapters/{name}/logs` - Get adapter logs

### MCP Proxy
- `POST /adapters/{name}/mcp` - Forward MCP streamable HTTP requests
- `POST /adapters/{name}/messages` - Forward MCP messages
- `GET /adapters/{name}/sse` - Forward Server-Sent Events

### API Documentation
- `GET /docs` - Interactive Swagger UI
- `GET /swagger/doc.json` - OpenAPI JSON specification

## Monitoring

Enable Prometheus monitoring:

```yaml
monitoring:
  enabled: true
  serviceMonitor:
    enabled: true
```

## Upgrading

```bash
helm upgrade mcp-gateway ./deployment/helm/mcp-gateway -f values-prod.yaml
```

## Uninstalling

```bash
helm uninstall mcp-gateway
```

## Troubleshooting

### Check pod status
```bash
kubectl get pods -l app.kubernetes.io/name=mcp-gateway
```

### View logs
```bash
kubectl logs -l app.kubernetes.io/name=mcp-gateway
```

### Check services
```bash
kubectl get services -l app.kubernetes.io/name=mcp-gateway
```

## Contributing

Please refer to the main project documentation for contribution guidelines.