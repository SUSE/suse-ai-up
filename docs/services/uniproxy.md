# SUSE AI Uniproxy Service

## Overview

The Uniproxy service is the main MCP (Model Context Protocol) proxy service that provides REST and JSON-RPC endpoints for managing MCP servers and adapters. It serves as the primary interface for AI assistants and applications to interact with MCP servers.

## Architecture

The Uniproxy service runs on port 8911 (HTTP) and 38911 (HTTPS) and provides:

- **REST API**: Standard HTTP endpoints for adapter and registry management
- **MCP Proxy**: JSON-RPC 2.0 proxy for MCP protocol communication
- **Authentication**: Multiple authentication methods for secure access
- **Session Management**: MCP session lifecycle management
- **Health Monitoring**: Service health and metrics endpoints

## Core Components

### MCP Protocol Handler
- Implements JSON-RPC 2.0 communication
- Manages MCP message routing between clients and servers
- Handles protocol versioning and capability negotiation

### Adapter Manager
- Manages MCP server adapters and their configurations
- Supports multiple connection types (LocalStdio, RemoteHttp, StreamableHttp, SSE)
- Provides adapter lifecycle management (create, update, delete)

### Session Store
- Maintains MCP session state and metadata
- Tracks active connections and capabilities
- Manages session cleanup and expiration

### Authentication System
- Supports Bearer tokens, Basic auth, and API keys
- Configurable per adapter
- Integration with external identity providers

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8911` | HTTP server port |
| `AUTH_MODE` | `development` | Authentication mode (development/production) |
| `OTEL_ENABLED` | `false` | Enable OpenTelemetry tracing |
| `OTEL_ENDPOINT` | - | OpenTelemetry collector endpoint |

### Command Line Options

```bash
./suse-ai-up uniproxy [flags]

Flags:
  -config string   Configuration file path
  -port int        Server port (default 8911)
  -debug           Enable debug logging
```

## API Endpoints

### Health & Monitoring
- `GET /health` - Service health check
- `GET /docs/*` - Swagger documentation
- `GET /swagger/doc.json` - OpenAPI specification

### Adapter Management
- `GET /api/v1/adapters` - List all adapters
- `POST /api/v1/adapters` - Create new adapter
- `GET /api/v1/adapters/{name}` - Get adapter details
- `PUT /api/v1/adapters/{name}` - Update adapter
- `DELETE /api/v1/adapters/{name}` - Delete adapter

### MCP Protocol Endpoints
- `ANY /api/v1/adapters/{name}/mcp` - MCP JSON-RPC proxy
- `GET /api/v1/adapters/{name}/tools` - List available tools
- `POST /api/v1/adapters/{name}/tools/{toolName}/call` - Call tool
- `GET /api/v1/adapters/{name}/resources` - List resources
- `GET /api/v1/adapters/{name}/resources/*uri` - Read resource
- `GET /api/v1/adapters/{name}/prompts` - List prompts
- `GET /api/v1/adapters/{name}/prompts/{promptName}` - Get prompt

### Session Management
- `GET /api/v1/adapters/{name}/sessions` - List sessions
- `POST /api/v1/adapters/{name}/sessions` - Create session
- `DELETE /api/v1/adapters/{name}/sessions` - Delete all sessions
- `GET /api/v1/adapters/{name}/sessions/{sessionId}` - Get session
- `DELETE /api/v1/adapters/{name}/sessions/{sessionId}` - Delete session

### Authentication
- `GET /api/v1/adapters/{name}/token` - Get adapter token
- `POST /api/v1/adapters/{name}/token/validate` - Validate token
- `POST /api/v1/adapters/{name}/token/refresh` - Refresh token
- `GET /api/v1/adapters/{name}/client-token` - Get client token
- `POST /api/v1/adapters/{name}/validate-auth` - Validate auth config
- `POST /api/v1/adapters/{name}/test-auth` - Test authentication

## Connection Types

### LocalStdio
For MCP servers running locally via stdio:

```json
{
  "connectionType": "LocalStdio",
  "mcpClientConfig": {
    "mcpServers": {
      "filesystem": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"],
        "env": {
          "NODE_ENV": "production"
        }
      }
    }
  }
}
```

### RemoteHttp
For remote MCP servers via HTTP:

```json
{
  "connectionType": "RemoteHttp",
  "apiBaseUrl": "https://api.example.com/mcp",
  "authentication": {
    "required": true,
    "type": "bearer",
    "bearerToken": {
      "token": "your-token"
    }
  }
}
```

### StreamableHttp
For servers supporting streaming responses:

```json
{
  "connectionType": "StreamableHttp",
  "apiBaseUrl": "https://streaming-api.example.com/mcp"
}
```

### SSE (Server-Sent Events)
For real-time streaming connections:

```json
{
  "connectionType": "SSE",
  "apiBaseUrl": "https://sse-api.example.com/mcp"
}
```

## VirtualMCP Support

The Uniproxy service includes special support for VirtualMCP adapters:

- Automatic reconfiguration for stdio communication
- Tool configuration via adapter metadata
- API base URL mapping for external integrations
- Bearer token authentication with long expiry

## Monitoring & Observability

### Health Checks
- Service availability monitoring
- Dependency health verification
- Automatic recovery mechanisms

### Metrics
- Request/response metrics
- MCP protocol statistics
- Session management metrics
- Error rate monitoring

### Logging
- Structured logging with configurable levels
- Request tracing and correlation IDs
- Error context and stack traces

## Security Features

### Authentication
- Multiple authentication methods
- Per-adapter security configuration
- Token expiration and refresh
- Secure credential storage

### Authorization
- Role-based access control
- Adapter-level permissions
- API endpoint restrictions
- Audit logging

### TLS/SSL
- HTTPS support with configurable certificates
- TLS 1.3 support
- Certificate rotation
- Secure header configuration

## Performance Optimization

### Caching
- MCP capability caching
- Session state caching
- Response caching for static resources

### Connection Pooling
- HTTP client connection reuse
- Database connection pooling
- Session connection management

### Rate Limiting
- Configurable rate limits per endpoint
- Burst handling and queue management
- Client-specific limits

## Deployment

### Docker
```yaml
FROM suse/suse-ai-up:latest
EXPOSE 8911
ENV AUTH_MODE=production
```

### Kubernetes
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: suse-ai-up
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: uniproxy
        image: suse/suse-ai-up:latest
        ports:
        - containerPort: 8911
        env:
        - name: AUTH_MODE
          value: "production"
```

### Helm Chart
```bash
helm install suse-ai-up ./helm/suse-ai-up \
  --set uniproxy.port=8911 \
  --set auth.mode=production
```

## Troubleshooting

### Common Issues

#### Connection Refused
- Check service port configuration
- Verify firewall settings
- Confirm service is running

#### Authentication Failures
- Validate token format and expiry
- Check authentication configuration
- Review adapter-specific auth settings

#### MCP Protocol Errors
- Verify MCP server compatibility
- Check protocol version support
- Review server logs for errors

### Debug Mode
Enable debug logging for detailed troubleshooting:

```bash
export LOG_LEVEL=debug
./suse-ai-up uniproxy -debug
```

### Health Checks
Monitor service health:

```bash
# Service health
curl http://localhost:8911/health

# Adapter status
curl http://localhost:8911/api/v1/adapters/my-adapter/status

# Session status
curl http://localhost:8911/api/v1/adapters/my-adapter/sessions
```

## Integration Examples

### AI Assistant Integration
```python
import requests

class MCPClient:
    def __init__(self, base_url="http://localhost:8911"):
        self.base_url = base_url

    def list_tools(self, adapter_name):
        response = requests.get(f"{self.base_url}/api/v1/adapters/{adapter_name}/tools")
        return response.json()

    def call_tool(self, adapter_name, tool_name, arguments):
        response = requests.post(
            f"{self.base_url}/api/v1/adapters/{adapter_name}/tools/{tool_name}/call",
            json=arguments
        )
        return response.json()
```

### Webhook Integration
```javascript
const axios = require('axios');

async function createAdapter(adapterConfig) {
    try {
        const response = await axios.post(
            'http://localhost:8911/api/v1/adapters',
            adapterConfig,
            {
                headers: {
                    'Content-Type': 'application/json'
                }
            }
        );
        return response.data;
    } catch (error) {
        console.error('Failed to create adapter:', error.response.data);
        throw error;
    }
}
```

## Best Practices

### Configuration Management
- Use environment variables for sensitive data
- Implement configuration validation
- Version control configuration files

### Monitoring
- Set up comprehensive monitoring
- Configure alerts for critical metrics
- Implement log aggregation

### Security
- Use HTTPS in production
- Implement proper authentication
- Regularly rotate credentials
- Monitor for security vulnerabilities

### Performance
- Configure appropriate resource limits
- Implement caching strategies
- Monitor performance metrics
- Optimize database queries

## API Versioning

The Uniproxy service uses API versioning with the `/api/v1/` prefix. Future versions will maintain backward compatibility and provide migration guides for breaking changes.