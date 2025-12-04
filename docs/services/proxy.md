# MCP Proxy Service

The MCP Proxy Service is the primary entry point for the SUSE AI Universal Proxy system. It provides a comprehensive HTTP proxy for Model Context Protocol (MCP) servers, enabling secure, scalable, and authenticated communication with AI models and services.

## Overview

The proxy service acts as a gateway between MCP clients and MCP servers, providing:

- **Full MCP Protocol Support**: Complete JSON-RPC 2.0 implementation
- **Multiple Transport Types**: HTTP, SSE (Server-Sent Events), WebSocket
- **Advanced Session Management**: Isolated sessions with automatic cleanup
- **Authentication Integration**: Support for multiple auth methods
- **Load Balancing**: Intelligent request distribution
- **Health Monitoring**: Real-time service health tracking

## Architecture

### Service Position
```
┌─────────────┐
│   CLIENT    │
│  (Browser,  │
│   CLI, IDE) │
└──────┬──────┘
       │
       ▼
┌─────────────┐    ┌─────────────┐
│   PROXY     │◄──►│  REGISTRY   │
│  (Primary)  │    │ (Sidecar)   │
│             │    │             │
│ Port: 8080  │    │ Port: 8913  │
│ HTTPS:38080 │    │ HTTPS:38913 │
└──────┬──────┘    └─────────────┘
       │
       ▼
┌─────────────┐
│ MCP SERVER  │
│  (External) │
└─────────────┘
```

### Key Components

- **HTTP Handler**: Routes MCP requests to appropriate servers
- **Session Manager**: Maintains client-server session state
- **Protocol Translator**: Converts between transport types
- **Authentication Middleware**: Validates client credentials
- **Load Balancer**: Distributes requests across server instances

## Configuration

### Environment Variables

```bash
# Basic Configuration
PROXY_PORT=8080              # HTTP port (default: 8080)
TLS_PORT=38080              # HTTPS port (default: 38080)

# TLS Configuration
AUTO_TLS=true               # Auto-generate self-signed certificates
TLS_CERT_FILE=/path/to/cert.pem
TLS_KEY_FILE=/path/to/key.pem

# Performance Tuning
MAX_CONNECTIONS=1000        # Maximum concurrent connections
SESSION_TIMEOUT=3600        # Session timeout in seconds
REQUEST_TIMEOUT=30          # Request timeout in seconds

# Authentication
AUTH_METHOD=oauth           # oauth, bearer, apikey, basic
AUTH_ENDPOINT=https://auth.example.com
CLIENT_ID=your-client-id
CLIENT_SECRET=your-client-secret
```

### Docker Configuration

```yaml
services:
  proxy:
    image: suse/suse-ai-up:latest
    ports:
      - "8080:8080"      # HTTP
      - "38080:38080"    # HTTPS
    environment:
      - AUTO_TLS=true
      - AUTH_METHOD=oauth
    command: ["./suse-ai-up", "proxy"]
```

### Kubernetes Configuration

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: suse-ai-up-proxy
spec:
  template:
    spec:
      containers:
      - name: proxy
        image: suse/suse-ai-up:latest
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 38080
          name: https
        env:
        - name: AUTO_TLS
          value: "true"
        - name: AUTH_METHOD
          value: "oauth"
        command: ["./suse-ai-up", "proxy"]
```

## API Endpoints

### MCP Protocol Endpoints

#### Initialize Connection
```http
POST /mcp
Content-Type: application/json

{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": "2024-11-05",
    "capabilities": {},
    "clientInfo": {
      "name": "my-client",
      "version": "1.0"
    }
  }
}
```

#### List Available Tools
```http
POST /mcp/tools
Content-Type: application/json

{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/list",
  "params": {}
}
```

#### Call a Tool
```http
POST /mcp/tools/{tool_name}
Content-Type: application/json

{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "tool_name",
    "arguments": {
      "arg1": "value1"
    }
  }
}
```

#### List Resources
```http
POST /mcp/resources
Content-Type: application/json

{
  "jsonrpc": "2.0",
  "id": 4,
  "method": "resources/list",
  "params": {}
}
```

#### Read Resource
```http
POST /mcp/resources/{resource_uri}
Content-Type: application/json

{
  "jsonrpc": "2.0",
  "id": 5,
  "method": "resources/read",
  "params": {
    "uri": "resource_uri"
  }
}
```

### Administrative Endpoints

#### Health Check
```http
GET /health
```

Response:
```json
{
  "service": "proxy",
  "status": "healthy",
  "timestamp": "2025-12-04T12:00:00Z",
  "version": "1.0.0"
}
```

## Authentication Integration

### OAuth 2.0 Flow

1. **Client Registration**: Register your application with the OAuth provider
2. **Authorization Request**: Redirect users to the authorization endpoint
3. **Token Exchange**: Exchange authorization code for access token
4. **API Requests**: Include Bearer token in MCP requests

```bash
# Configure OAuth
export AUTH_METHOD=oauth
export AUTH_ENDPOINT=https://auth.example.com
export CLIENT_ID=your-client-id
export CLIENT_SECRET=your-client-secret
```

### Bearer Token Authentication

```bash
# Configure Bearer tokens
export AUTH_METHOD=bearer
export BEARER_TOKEN=your-jwt-token
```

### API Key Authentication

```bash
# Configure API keys
export AUTH_METHOD=apikey
export API_KEY=your-api-key
export API_KEY_HEADER=X-API-Key
```

## Transport Types

### HTTP Transport
Standard HTTP POST requests with JSON-RPC payloads.

**Pros**: Simple, widely supported
**Cons**: Client must poll for responses

### Server-Sent Events (SSE)
Real-time streaming responses using EventSource API.

**Pros**: Real-time updates, efficient
**Cons**: Requires SSE-capable clients

### WebSocket Transport
Bidirectional communication with persistent connections.

**Pros**: Lowest latency, full duplex
**Cons**: More complex client implementation

## Session Management

### Session Lifecycle

1. **Initialization**: Client sends `initialize` request
2. **Session Creation**: Proxy creates isolated session context
3. **Request Processing**: All subsequent requests use session context
4. **Cleanup**: Automatic cleanup on disconnect or timeout

### Session Configuration

```bash
# Session settings
export SESSION_TIMEOUT=3600        # 1 hour timeout
export MAX_SESSIONS_PER_CLIENT=10 # Limit concurrent sessions
export SESSION_CLEANUP_INTERVAL=300 # Cleanup every 5 minutes
```

### Session Isolation

- **Security**: Sessions are completely isolated
- **Resource Limits**: Per-session resource quotas
- **Timeout Management**: Automatic cleanup of stale sessions
- **Monitoring**: Session metrics and health tracking

## Load Balancing

### Server Discovery

The proxy integrates with the Registry service to discover available MCP servers:

```bash
# Query registry for servers
curl http://localhost:8913/api/v1/registry/browse?transport=http
```

### Load Balancing Strategies

- **Round Robin**: Distribute requests evenly
- **Least Connections**: Route to least loaded server
- **Health-Based**: Avoid unhealthy servers
- **Geographic**: Route based on client location

### Configuration

```yaml
loadBalancer:
  strategy: round_robin
  healthCheck:
    enabled: true
    interval: 30s
    timeout: 5s
  servers:
    - url: "http://mcp-server-1:8080"
      weight: 1
    - url: "http://mcp-server-2:8080"
      weight: 2
```

## Monitoring & Observability

### Metrics

The proxy exposes Prometheus-compatible metrics:

```
# Request rate
mcp_proxy_requests_total{method="tools/call", status="200"} 150

# Response latency
mcp_proxy_request_duration_seconds{quantile="0.5"} 0.023

# Active sessions
mcp_proxy_active_sessions 5

# Error rate
mcp_proxy_errors_total{type="timeout"} 2
```

### Health Checks

- **Readiness Probe**: Checks if proxy can accept requests
- **Liveness Probe**: Detects if proxy needs restart
- **Dependency Checks**: Validates registry connectivity

### Logging

Structured logging with configurable levels:

```json
{
  "timestamp": "2025-12-04T12:00:00Z",
  "level": "info",
  "service": "proxy",
  "session_id": "sess_123",
  "method": "tools/call",
  "duration_ms": 23,
  "status": 200
}
```

## Performance Tuning

### Connection Pooling

```bash
export MAX_IDLE_CONNECTIONS=100
export MAX_IDLE_CONNECTIONS_PER_HOST=10
export IDLE_CONNECTION_TIMEOUT=90s
```

### Request Timeouts

```bash
export REQUEST_TIMEOUT=30s
export DIAL_TIMEOUT=10s
export TLS_HANDSHAKE_TIMEOUT=10s
```

### Resource Limits

```yaml
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 512Mi
```

## Troubleshooting

### Common Issues

**Connection Refused**
```bash
# Check if registry service is running
curl http://localhost:8913/health

# Verify network connectivity
telnet localhost 8913
```

**Authentication Errors**
```bash
# Validate auth configuration
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/health

# Check auth service logs
kubectl logs -f deployment/suse-ai-up-auth
```

**High Latency**
```bash
# Check server response times
curl -w "@curl-format.txt" http://localhost:8080/health

# Monitor resource usage
kubectl top pods
```

### Debug Mode

Enable detailed logging for troubleshooting:

```bash
export LOG_LEVEL=debug
export LOG_FORMAT=json
./suse-ai-up proxy
```

### Health Validation

```bash
# Comprehensive health check
curl http://localhost:8911/health

# Individual service checks
curl http://localhost:8080/health    # Proxy
curl http://localhost:8913/health    # Registry
curl http://localhost:8912/health    # Discovery
curl http://localhost:8914/health    # Plugins
```

## Security Considerations

### TLS Configuration

- **Production**: Always use proper certificates
- **Development**: Auto-generated certificates acceptable
- **Validation**: Certificate pinning recommended

### Authentication Best Practices

- **Token Rotation**: Implement regular token refresh
- **Scope Limitation**: Use minimal required permissions
- **Audit Logging**: Log all authentication events

### Network Security

- **Firewall Rules**: Restrict access to necessary ports
- **Rate Limiting**: Implement request rate limits
- **DDoS Protection**: Use CDN or load balancer protection

## Integration Examples

### JavaScript Client

```javascript
import { MCPClient } from 'mcp-client';

const client = new MCPClient({
  endpoint: 'http://localhost:8080',
  auth: {
    type: 'bearer',
    token: 'your-jwt-token'
  }
});

// Initialize connection
await client.initialize();

// List available tools
const tools = await client.listTools();

// Call a tool
const result = await client.callTool('calculator', {
  expression: '2 + 2'
});
```

### Python Client

```python
from mcp_client import MCPClient

client = MCPClient(
    endpoint="http://localhost:8080",
    auth={"type": "apikey", "key": "your-api-key"}
)

# Initialize
client.initialize()

# Use tools
tools = client.list_tools()
result = client.call_tool("weather", {"location": "Berlin"})
```

### cURL Examples

```bash
# Initialize session
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}'

# List tools
curl -X POST http://localhost:8080/mcp/tools \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
```

## Migration Guide

### From Direct MCP Connections

**Before:**
```
Client → MCP Server (direct connection)
```

**After:**
```
Client → Proxy → Registry → MCP Server
```

### Configuration Migration

1. **Update Endpoints**: Change client configurations to point to proxy
2. **Add Authentication**: Configure appropriate auth method
3. **Update Timeouts**: Adjust timeouts for proxy overhead
4. **Test Gradually**: Migrate clients incrementally

### Compatibility Matrix

| MCP Version | Transport | Authentication | Status |
|-------------|-----------|----------------|--------|
| 2024-11-05  | HTTP      | OAuth 2.0     | ✅ Full |
| 2024-11-05  | SSE       | Bearer        | ✅ Full |
| 2024-11-05  | WebSocket | API Key       | ✅ Full |
| 2024-10-01  | HTTP      | Basic         | ⚠️ Deprecated |

## Advanced Configuration

### Custom Middleware

```go
// Add custom middleware
proxy.UseMiddleware(func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Custom logic here
        next.ServeHTTP(w, r)
    })
})
```

### Custom Transports

```go
// Register custom transport
proxy.RegisterTransport("custom", &CustomTransport{})
```

### Plugin Integration

```go
// Load custom plugins
proxy.LoadPlugin("custom-auth", &CustomAuthPlugin{})
```

This comprehensive proxy service provides a robust, scalable, and secure gateway for MCP communications while maintaining full protocol compatibility and extensibility.