# Basic Proxy Example

This example demonstrates a minimal SUSE AI Universal Proxy deployment with just the proxy service running. It's perfect for getting started quickly or for simple use cases where you just need basic MCP proxying.

## Overview

The basic proxy setup includes:
- Single proxy service
- Auto-generated TLS certificates
- Development authentication mode
- Minimal configuration

## Quick Start

### Using Docker

```bash
# Run the basic proxy
docker run -p 8080:8080 -p 38080:38080 \
  -e AUTH_MODE=development \
  -e AUTO_TLS=true \
  suse/suse-ai-up:latest ./suse-ai-up proxy
```

### Using Docker Compose

```yaml
# docker-compose.yml
version: '3.8'
services:
  proxy:
    image: suse/suse-ai-up:latest
    ports:
      - "8080:8080"      # HTTP
      - "38080:38080"    # HTTPS
    environment:
      - AUTH_MODE=development
      - AUTO_TLS=true
    command: ["./suse-ai-up", "proxy"]
```

```bash
docker-compose up -d
```

## Configuration

### Environment Variables

```bash
# Basic Configuration
PROXY_PORT=8080              # HTTP port
TLS_PORT=38080               # HTTPS port

# TLS Configuration
AUTO_TLS=true                # Auto-generate certificates
TLS_CERT_FILE=               # Optional: custom certificate
TLS_KEY_FILE=                # Optional: custom key

# Authentication
AUTH_MODE=development        # development, bearer, oauth, none

# Performance
MAX_CONNECTIONS=1000         # Maximum concurrent connections
REQUEST_TIMEOUT=30           # Request timeout in seconds
SESSION_TIMEOUT=3600         # Session timeout in seconds
```

### Configuration File

```yaml
# config.yaml
proxy:
  port: 8080
  tlsPort: 38080
  autoTLS: true

auth:
  mode: "development"

logging:
  level: "info"
  format: "json"
```

## Usage Examples

### Basic MCP Request

```bash
# Health check
curl http://localhost:8080/health

# List available tools (if MCP server configured)
curl http://localhost:8080/mcp/tools \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}'
```

### HTTPS with Self-Signed Certificate

```bash
# Accept self-signed certificate
curl -k https://localhost:38080/health

# Or add certificate to trust store
curl https://localhost:38080/health
```

## Connecting MCP Servers

### Manual Server Registration

```bash
# Register an MCP server
curl -X POST http://localhost:8080/api/v1/registry/upload \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Example MCP Server",
    "description": "A sample MCP server",
    "packages": [{
      "name": "example-tools",
      "transport": {
        "type": "http",
        "url": "http://mcp-server:8080"
      }
    }]
  }'
```

### Environment-Based Configuration

```bash
# Set MCP server endpoint
export MCP_SERVER_URL=http://mcp-server:8080

# Configure proxy to use the server
export DEFAULT_MCP_SERVER=$MCP_SERVER_URL
```

## Development Setup

### Local Development

```bash
# Clone the repository
git clone https://github.com/suse/suse-ai-up.git
cd suse-ai-up

# Build the binary
go build -o suse-ai-up ./cmd

# Run in development mode
./suse-ai-up proxy
```

### IDE Integration

```json
// .vscode/launch.json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Debug Proxy",
      "type": "go",
      "request": "launch",
      "mode": "debug",
      "program": "${workspaceFolder}/cmd",
      "args": ["proxy"],
      "env": {
        "AUTH_MODE": "development",
        "AUTO_TLS": "true"
      }
    }
  ]
}
```

## Testing

### Basic Connectivity Test

```bash
#!/bin/bash
# test-basic.sh

echo "Testing basic proxy connectivity..."

# Test HTTP endpoint
if curl -f http://localhost:8080/health > /dev/null 2>&1; then
    echo "✅ HTTP endpoint is accessible"
else
    echo "❌ HTTP endpoint is not accessible"
fi

# Test HTTPS endpoint (accept self-signed cert)
if curl -k -f https://localhost:38080/health > /dev/null 2>&1; then
    echo "✅ HTTPS endpoint is accessible"
else
    echo "❌ HTTPS endpoint is not accessible"
fi

echo "Basic connectivity test complete."
```

### MCP Protocol Test

```bash
#!/bin/bash
# test-mcp.sh

echo "Testing MCP protocol..."

# Test MCP initialize
response=$(curl -s http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {},
      "clientInfo": {
        "name": "test-client",
        "version": "1.0.0"
      }
    }
  }')

if echo "$response" | jq -e '.result' > /dev/null 2>&1; then
    echo "✅ MCP protocol is working"
else
    echo "❌ MCP protocol test failed"
    echo "Response: $response"
fi
```

## Troubleshooting

### Common Issues

**Port Already in Use**
```bash
# Check what's using the ports
lsof -i :8080
lsof -i :38080

# Use different ports
export PROXY_PORT=8081
export TLS_PORT=38081
```

**TLS Certificate Errors**
```bash
# Regenerate certificates
rm -f tls.crt tls.key
export AUTO_TLS=true

# Or disable TLS for testing
export AUTO_TLS=false
```

**Connection Refused**
```bash
# Check if service is running
ps aux | grep suse-ai-up

# Check logs
docker logs <container-id>

# Verify configuration
curl http://localhost:8080/health
```

### Debug Mode

Enable detailed logging:

```bash
export LOG_LEVEL=debug
export LOG_FORMAT=json
./suse-ai-up proxy
```

## Next Steps

Once you have the basic proxy working, consider:

1. **Add Authentication**: Switch from development mode to proper authentication
2. **Configure Registry**: Add the registry service for server discovery
3. **Enable Monitoring**: Add health checks and metrics
4. **Production Deployment**: Use the full-stack example for production

## Related Examples

- [Full Stack Example](../full-stack/): Complete deployment with all services
- [Kubernetes Example](../kubernetes/): Production Kubernetes deployment
- [Custom Configuration](../../docs/configuration.md): Advanced configuration options</content>
<parameter name="filePath">examples/basic-proxy/README.md