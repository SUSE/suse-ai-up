# MCP Server Adapters

MCP (Model Context Protocol) server adapters provide a unified interface for managing and proxying requests to MCP servers deployed across different environments. Adapters handle authentication, load balancing, health monitoring, and protocol translation.

## Overview

The adapter system enables seamless integration of discovered MCP servers into the SUSE AI Universal Proxy infrastructure. Once an MCP server is discovered through the [network discovery system](discovery.md), it can be registered as an adapter for production use.

## Architecture

### Components

- **Adapter Resource**: Kubernetes-native representation of an MCP server deployment
- **Proxy Handler**: Routes MCP requests to appropriate adapter instances
- **Health Monitoring**: Continuous health checks and status reporting
- **Authentication Middleware**: Enforces access controls and authentication flows
- **Session Management**: Tracks active connections and manages session lifecycle

### Deployment Types

The system supports multiple adapter deployment types:

| Type | Description | Use Case |
|------|-------------|----------|
| **StreamableHttp** | HTTP-based MCP servers with streaming support | Most MCP servers |
| **RemoteHttp** | External HTTP MCP servers | Third-party services |
| **LocalStdio** | Local MCP servers via stdio | Development, testing |
| **SSE** | Server-Sent Events based servers | Real-time streaming |

## API Usage

### Register Discovered Server as Adapter

After discovering MCP servers, register them as adapters:

```bash
POST /api/v1/register
Content-Type: application/json

{
  "discoveredServerId": "mcp-192-168-1-100-8000-123456789"
}
```

**Success Response** (Normal Server):
```json
{
  "message": "Adapter successfully created from discovered server",
  "discoveredServer": {
    "id": "mcp-192-168-1-100-8000-123456789",
    "name": "MCP Server at http://192.168.1.100:8000",
    "address": "http://192.168.1.100:8000",
    "protocol": "MCP",
    "connection": "StreamableHttp",
    "vulnerability_score": "medium"
  },
  "adapter": {
    "id": "discovered-192-168-1-100-123456789",
    "name": "discovered-192-168-1-100-123456789"
  }
}
```

**Success Response** (High-Risk Server - Auto-Secured):
```json
{
  "message": "Adapter successfully created from discovered server",
  "discoveredServer": {
    "id": "mcp-192-168-1-100-8000-123456789",
    "name": "MCP Server at http://192.168.1.100:8000",
    "address": "http://192.168.1.100:8000",
    "protocol": "MCP",
    "connection": "StreamableHttp",
    "vulnerability_score": "high",
    "metadata": {
      "auth_type": "none"
    }
  },
  "adapter": {
    "id": "discovered-192-168-1-100-123456789",
    "name": "discovered-192-168-1-100-123456789"
  },
  "security": {
    "enhanced": true,
    "auth_type": "bearer",
    "token_required": true,
    "note": "High-risk server automatically secured with bearer authentication"
  }
}
```

### List All Adapters

```bash
GET /api/v1/adapters
```

**Response**:
```json
[
  {
    "id": "discovered-192-168-1-100-123456789",
    "name": "discovered-192-168-1-100-123456789",
    "protocol": "MCP",
    "connectionType": "StreamableHttp",
    "description": "Auto-discovered MCP server at http://192.168.1.100:8000",
    "createdBy": "user@example.com",
    "createdAt": "2024-11-05T10:30:00Z",
    "lastUpdatedAt": "2024-11-05T10:30:00Z"
  }
]
```

### Get Adapter Details

```bash
GET /api/v1/adapters/{name}
```

### Update Adapter Configuration

```bash
PUT /api/v1/adapters/{name}
Content-Type: application/json

{
  "replicaCount": 3,
  "environmentVariables": {
    "MCP_DEBUG": "true"
  }
}
```

### Delete Adapter

```bash
DELETE /api/v1/adapters/{name}
```

### Check Adapter Status

```bash
GET /api/v1/adapters/{name}/status
```

**Response**:
```json
{
  "readyReplicas": [1],
  "updatedReplicas": [1],
  "availableReplicas": [1],
  "image": "mcp-proxy:1.0.0",
  "replicaStatus": "Healthy"
}
```

### Get Adapter Logs

```bash
GET /api/v1/adapters/{name}/logs?instance=0
```

## Automatic Security Enhancement

### High-Risk Server Protection

When registering discovered MCP servers marked as **high-risk** (no authentication), the system automatically adds **bearer token authentication** to secure the adapter proxy layer.

**Detection Criteria**:
- `vulnerability_score: "high"`
- `auth_type: "none"` (no authentication on the backend MCP server)

**Automatic Actions**:
1. **Generates Secure Token**: Creates cryptographically secure 256-bit random token
2. **Enforces Authentication**: All API calls to the adapter require `Authorization: Bearer <token>`
3. **Updates Description**: Adds security notice to adapter description
4. **Maintains Compatibility**: Backend MCP server remains unchanged

### Security Benefits

**Before Registration** (High-Risk):
```
Client → MCP Server (no auth required)
```

**After Registration** (Auto-Secured):
```
Client → Adapter (Bearer auth required) → MCP Server (no auth)
```

### API Usage with Authentication

**Without Token** (401 Unauthorized):
```bash
curl http://localhost:8911/api/v1/adapters/discovered-secure-mcp/mcp
# Returns: {"code":"MISSING_AUTH_HEADER","message":"Authentication required"}
```

**With Token** (200 OK):
```bash
curl -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
  http://localhost:8911/api/v1/adapters/discovered-secure-mcp/mcp
```

### Token Management

- **Token Storage**: Securely stored in adapter configuration
- **Token Access**: Available through adapter details API
- **Token Security**: 256-bit cryptographically secure random generation
- **No Expiration**: Tokens remain valid until adapter is deleted or reconfigured

## Adapter Naming Convention

When registering discovered servers, adapters are automatically named using the pattern:

```
discovered-{sanitized-host}-{timestamp}
```

**Examples**:
- `192.168.1.100:8000` → `discovered-192-168-1-100-123456789`
- `mcp.example.com:9000` → `discovered-mcp-example-com-123456789`

## Configuration Mapping

### From Discovered Server to Adapter

The system automatically maps discovered server properties to adapter configuration:

| Discovered Property | Adapter Property | Mapping Logic |
|-------------------|------------------|---------------|
| `address` | `remoteUrl` / `environmentVariables` | RemoteHttp uses `remoteUrl`, others use env var |
| `connection` | `connectionType` | Direct mapping with validation |
| `protocol` | `protocol` | Always `ServerProtocolMCP` |
| `metadata.auth_type` | `authentication` | Maps to appropriate auth config |
| Server name | `description` | Auto-generated description |

### Connection Type Specifics

#### StreamableHttp Adapters
- **Deployment**: Kubernetes StatefulSet with service mesh
- **Image**: `mcp-proxy:1.0.0` (configurable)
- **Environment**: `MCP_PROXY_URL` set to discovered server address
- **Scaling**: Supports horizontal scaling with load balancing

#### RemoteHttp Adapters
- **Deployment**: Lightweight proxy configuration
- **Remote URL**: Direct connection to discovered server
- **Authentication**: Passthrough or adapter-level auth
- **No Kubernetes Deployment**: Proxy-only configuration

#### LocalStdio Adapters
- **Deployment**: Local process management
- **Command**: Auto-detected or configured
- **Arguments**: MCP client configuration
- **Development Use**: Primarily for testing and development

## Authentication Integration

### Automatic Auth Configuration

Based on discovered authentication type, adapters are configured with appropriate security:

| Discovered Auth Type | Adapter Auth Config | Security Level |
|---------------------|-------------------|----------------|
| `oauth` | OAuth 2.1 middleware | High |
| `bearer` | Bearer token validation | Medium |
| `basic` | Basic auth (discouraged) | Low |
| `none` | Optional auth | Variable |

### Session Management

Adapters support session-based authentication:

```bash
# Create session
POST /api/v1/adapters/{name}/sessions

# List active sessions
GET /api/v1/adapters/{name}/sessions

# Delete all sessions
DELETE /api/v1/adapters/{name}/sessions
```

## Health Monitoring

### Automatic Health Checks

- **Interval**: Configurable (default: 30 seconds)
- **Protocol**: MCP initialize requests
- **Failure Threshold**: 3 consecutive failures
- **Recovery**: Automatic when healthy responses resume

### Status Reporting

Health status is reported through the adapter status endpoint:

```json
{
  "status": "healthy",
  "lastChecked": "2024-11-05T10:35:00Z",
  "responseTime": 150000000,
  "message": "Adapter responding normally"
}
```

## Scaling and Performance

### Horizontal Scaling

StreamableHttp adapters support horizontal scaling:

```bash
PUT /api/v1/adapters/{name}
{
  "replicaCount": 5
}
```

### Resource Management

Default resource allocations:
- **CPU**: 250m request, 1 core limit
- **Memory**: 256Mi request, 512Mi limit
- **Storage**: None (stateless by default)

### Load Balancing

- **Algorithm**: Round-robin across healthy replicas
- **Session Affinity**: Optional sticky sessions
- **Health-Based Routing**: Automatic failover from unhealthy instances

## Error Handling

### Registration Failures

Common registration failure scenarios:

| Error | Cause | Resolution |
|-------|-------|------------|
| `Adapter already exists` | Name conflict | Use different discovered server or delete existing |
| `Invalid server address` | Malformed URL | Verify discovered server data |
| `Kubernetes deployment failed` | Cluster issues | Check cluster status and permissions |
| `Authentication required` | Missing user context | Ensure proper authentication headers |

### Runtime Errors

| Error | Cause | Resolution |
|-------|-------|------------|
| `Connection refused` | Target server down | Check discovered server availability |
| `Authentication failed` | Invalid credentials | Update authentication configuration |
| `Timeout` | Network issues | Increase timeout values or check connectivity |
| `Resource exhausted` | High load | Scale adapter replicas or increase resources |

## Integration with Discovery

### Discovery-to-Adapter Workflow

1. **Network Scan**: Discover MCP servers across infrastructure
2. **Server Validation**: Verify MCP protocol compliance
3. **Auth Assessment**: Determine security requirements
4. **Adapter Registration**: Create production-ready adapter
5. **Deployment**: Launch Kubernetes resources
6. **Health Monitoring**: Continuous status tracking
7. **Load Balancing**: Distribute requests across instances

### Automated Registration

For bulk operations, discovered servers can be registered programmatically:

```bash
#!/bin/bash
# Get discovered servers
SERVERS=$(curl -s http://localhost:8911/api/v1/servers)

# Register each server as adapter
echo "$SERVERS" | jq -r '.[].id' | while read serverId; do
  curl -X POST http://localhost:8911/api/v1/register \
    -H "Content-Type: application/json" \
    -d "{\"discoveredServerId\": \"$serverId\"}"
done
```

## Security Considerations

### Access Control

- **RBAC Integration**: Kubernetes role-based access control
- **Adapter-Level Auth**: Per-adapter authentication policies
- **Audit Logging**: All adapter operations are logged
- **Network Policies**: Kubernetes network segmentation

### Vulnerability Management

- **Auto-Assessment**: Vulnerability scoring from discovery
- **Security Headers**: Automatic security header injection
- **TLS Enforcement**: Required for production deployments
- **Secret Management**: Secure credential storage

## Monitoring and Observability

### Metrics

Adapters expose Prometheus metrics:
- Request count and latency
- Error rates by type
- Health check status
- Resource utilization

### Logging

- **Structured Logs**: JSON-formatted log entries
- **Request Tracing**: Correlation IDs across requests
- **Error Context**: Detailed error information
- **Audit Trail**: Security-relevant operations

## Troubleshooting

### Common Issues

**Adapter not responding**:
```bash
# Check adapter status
curl http://localhost:8911/api/v1/adapters/{name}/status

# Check adapter logs
curl http://localhost:8911/api/v1/adapters/{name}/logs

# Verify discovered server is still available
curl -I {original-server-address}/mcp
```

**Registration fails**:
```bash
# Check discovery service logs
tail -f proxy.log | grep RegisterServer

# Verify discovered server exists
curl http://localhost:8911/api/v1/servers | jq '.[] | select(.id == "server-id")'
```

**Authentication issues**:
```bash
# Check authentication configuration
curl http://localhost:8911/api/v1/adapters/{name}

# Test with authentication
curl -H "Authorization: Bearer {token}" \
  http://localhost:8911/api/v1/adapters/{name}/mcp
```

### Debug Mode

Enable debug logging for detailed troubleshooting:

```bash
# Set environment variable
export LOG_LEVEL=debug

# Restart service
kill %1 && go run cmd/service/main.go
```

## Examples

### Complete Discovery-to-Adapter Workflow

```bash
# 1. Start network scan
SCAN_ID=$(curl -X POST http://localhost:8911/api/v1/scan \
  -H "Content-Type: application/json" \
  -d '{"scanRanges": ["192.168.1.0/24"], "ports": [8000]}' \
  | jq -r '.scanId')

# 2. Wait for scan completion
while [ "$(curl -s http://localhost:8911/api/v1/scan/$SCAN_ID | jq -r '.status')" != "completed" ]; do
  sleep 2
done

# 3. Get discovered servers
SERVERS=$(curl -s http://localhost:8911/api/v1/servers)

# 4. Register first server as adapter
SERVER_ID=$(echo "$SERVERS" | jq -r '.[0].id')
curl -X POST http://localhost:8911/api/v1/register \
  -H "Content-Type: application/json" \
  -d "{\"discoveredServerId\": \"$SERVER_ID\"}"

# 5. Verify adapter creation
curl http://localhost:8911/api/v1/adapters

# 6. Test adapter functionality
curl -X POST http://localhost:8911/api/v1/adapters/discovered-192-168-1-100-*/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'
```

### Custom Adapter Configuration

```bash
# Register with custom configuration
curl -X POST http://localhost:8911/api/v1/register \
  -H "Content-Type: application/json" \
  -d '{
    "discoveredServerId": "mcp-server-123",
    "customConfig": {
      "replicaCount": 3,
      "environmentVariables": {
        "MCP_LOG_LEVEL": "debug",
        "MCP_TIMEOUT": "30s"
      }
    }
  }'
```

## Future Enhancements

- **Auto-Scaling**: Dynamic replica adjustment based on load
- **Blue-Green Deployments**: Zero-downtime adapter updates
- **Multi-Region Support**: Cross-region adapter replication
- **Advanced Routing**: Content-based request routing
- **Service Mesh Integration**: Istio and Linkerd support
- **Custom Resource Definitions**: Kubernetes-native adapter management</content>
</xai:function_call">## Integration with Discovery

The adapter system is tightly integrated with the [MCP Server Discovery](discovery.md) system. Discovered servers can be seamlessly registered as production-ready adapters with full lifecycle management.

### Discovery-to-Adapter Workflow

1. **Network Discovery**: Use the discovery system to find MCP servers
2. **Server Assessment**: Evaluate security posture and capabilities
3. **Adapter Registration**: Register discovered servers as adapters
4. **Production Deployment**: Launch managed Kubernetes deployments
5. **Monitoring & Scaling**: Full operational management

### Automated Registration

For bulk operations, combine discovery and adapter registration:

```bash
# Discover servers
curl -X POST http://localhost:8911/api/v1/scan \
  -H "Content-Type: application/json" \
  -d '{"scanRanges": ["10.0.0.0/8"], "ports": [8000, 8001]}'

# Register all discovered servers as adapters
curl -s http://localhost:8911/api/v1/servers | \
  jq -r '.[].id' | \
  xargs -I {} curl -X POST http://localhost:8911/api/v1/register \
    -H "Content-Type: application/json" \
    -d "{\"discoveredServerId\": \"{}\"}"
```

See the [Discovery Documentation](discovery.md) for detailed scanning and server detection procedures.