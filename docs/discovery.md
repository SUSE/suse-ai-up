# MCP Server Discovery

The MCP (Model Context Protocol) discovery system provides comprehensive network scanning capabilities to automatically detect and catalog MCP servers across your infrastructure.

## Overview

The discovery system can:
- Scan IP ranges and individual hosts for MCP servers
- Detect both authenticated and unauthenticated MCP servers
- Discriminate between different authentication types (OAuth, Bearer tokens, Basic auth, etc.)
- Provide security vulnerability assessments
- Support full CIDR range notation (/1 to /32 for IPv4)
- Automatically include localhost addresses for development

## Features

### Network Scanning
- **CIDR Range Support**: Full support for IPv4 CIDR notation (`192.168.1.0/24`, `10.0.0.0/8`, etc.)
- **Port Range Specification**: Scan multiple ports (`8000,8001,9000-9100`)
- **Concurrent Scanning**: Efficient parallel scanning with configurable concurrency limits
- **Safety Limits**: Prevents excessive resource usage with 65K IP limit per range

### Authentication Detection
The system intelligently detects and categorizes different authentication types:

| Auth Type | Detection Method | Vulnerability Score | Description |
|-----------|------------------|-------------------|-------------|
| **OAuth 2.1** | `WWW-Authenticate` header with `resource_metadata` | `low` | Enterprise-grade OAuth flows |
| **OpenID Connect** | OIDC indicators and endpoints | `low` | Identity layer with OAuth |
| **Bearer Token** | `Bearer` scheme in `WWW-Authenticate` | `medium` | API token authentication |
| **Basic Auth** | `Basic` scheme in `WWW-Authenticate` | `high` | Username/password (insecure) |
| **Digest Auth** | `Digest` scheme in `WWW-Authenticate` | `medium` | Hashed credential exchange |
| **Kerberos** | `Negotiate` scheme in `WWW-Authenticate` | `low` | Enterprise SSO |
| **API Key** | API key patterns in responses | `medium` | Custom key-based auth |
| **None** | No authentication required | `high` | Open access (vulnerable) |

### MCP Protocol Detection
- **JSON-RPC Detection**: Identifies valid MCP initialize responses
- **HTTP Auth Response Analysis**: Detects MCP servers via 401/403 error pages
- **Header Analysis**: Checks for MCP-specific headers and content
- **Server-Sent Events**: Supports SSE response format

## API Usage

### Start Network Scan

```bash
curl -X POST http://localhost:8911/scan \
  -H "Content-Type: application/json" \
  -d '{
    "scanRanges": ["192.168.1.0/24", "10.0.0.1-10.0.0.10"],
    "ports": [8000, 8001, 9000],
    "timeout": "30s",
    "maxConcurrent": 10
  }'
```

**Parameters**:
- `scanRanges`: Array of IP ranges in CIDR notation or IP ranges
- `ports`: Array of port numbers or port ranges
- `timeout`: Scan timeout per target (default: 30s)
- `maxConcurrent`: Maximum concurrent scan operations (default: 10)

### Check Scan Status

```bash
curl http://localhost:8911/scan/{scanId}
```

**Response**:
```json
{
  "scanId": "scan-1762180330654346000",
  "status": "completed",
  "serverCount": 2,
  "results": [
    {
      "id": "mcp-127-0-0-1-8001-1762180330654346000",
      "name": "MCP Server (Authenticated)",
      "address": "http://127.0.0.1:8001",
      "protocol": "MCP",
      "connection": "StreamableHttp",
      "status": "discovered",
      "vulnerability_score": "medium",
      "metadata": {
        "auth_type": "bearer",
        "auth_scheme": "Bearer",
        "auth_realm": "api",
        "detection_method": "http-auth-response"
      }
    }
  ]
}
```

### List All Discovered Servers

```bash
curl http://localhost:8911/servers
```

## Security Assessment

The discovery system provides vulnerability scoring based on authentication type:

- **`low`**: Enterprise-grade security (OAuth, OpenID, Kerberos)
- **`medium`**: Standard security practices (Bearer tokens, API keys, Digest)
- **`high`**: Inadequate security (Basic auth, no authentication)

## Automatic Localhost Inclusion

The discovery system automatically includes localhost addresses in all scans:
- `127.0.0.1/32`
- `localhost`

This ensures development and testing environments are always discoverable.

## Examples

### Basic Network Scan
```bash
curl -X POST http://localhost:8911/scan \
  -H "Content-Type: application/json" \
  -d '{"scanRanges": ["192.168.1.0/24"], "ports": [8000]}'
```

### Enterprise Multi-Range Scan
```bash
curl -X POST http://localhost:8911/scan \
  -H "Content-Type: application/json" \
  -d '{
    "scanRanges": [
      "10.0.0.0/8",
      "172.16.0.0/12",
      "192.168.0.0/16"
    ],
    "ports": [8000, 8001, 8002, 9000],
    "maxConcurrent": 20
  }'
```

### Development Scan
```bash
curl -X POST http://localhost:8911/scan \
  -H "Content-Type: application/json" \
  -d '{"scanRanges": ["127.0.0.1/32"], "ports": [8001, 8002, 8003]}'
```

## Architecture

### Components
- **DiscoveryService**: Main service handling scan operations
- **NetworkScanner**: Low-level network scanning utilities
- **CIDR Expansion**: Converts CIDR ranges to individual IP addresses
- **MCP Detection**: Protocol-specific server identification
- **Auth Analysis**: Authentication type classification

### Detection Flow
1. **CIDR Expansion**: Convert ranges to IP lists
2. **Concurrent Scanning**: Parallel HTTP requests to IP:port combinations
3. **MCP Protocol Check**: Send JSON-RPC initialize requests
4. **Response Analysis**: Parse responses for MCP indicators
5. **Auth Type Detection**: Analyze WWW-Authenticate headers and content
6. **Metadata Collection**: Gather comprehensive server information
7. **Vulnerability Scoring**: Assess security posture

## Error Handling

The system gracefully handles:
- Invalid CIDR ranges
- Network timeouts
- Unresponsive hosts
- Malformed responses
- Excessive range sizes (65K IP limit)

## Performance Considerations

- **Concurrent Limits**: Configurable parallelism to prevent resource exhaustion
- **Timeout Controls**: Per-target timeouts prevent hanging
- **Range Size Limits**: Safety caps on IP count per scan
- **Memory Management**: Streaming responses to handle large result sets

## Integration

The discovery system integrates with:
- **Management Service**: Register discovered servers as adapters
- **Authorization Service**: Configure appropriate auth flows
- **Registry Service**: Catalog discovered server capabilities
- **Monitoring**: Track server health and availability

## Troubleshooting

### Common Issues

**No servers found**:
- Verify IP ranges and ports are correct
- Check network connectivity
- Ensure MCP servers are running and accessible

**Authentication detection fails**:
- Some servers may not return standard WWW-Authenticate headers
- Custom authentication schemes may not be recognized

**Scan timeouts**:
- Large CIDR ranges may take significant time
- Increase timeout values for slow networks
- Reduce concurrent scan limits

### Debug Information

Enable detailed logging to troubleshoot issues:
```bash
# Check scan logs
tail -f proxy.log | grep DiscoveryService

# Manual server testing
curl -I http://target-host:port/mcp
```

## Testing the Discovery System

The discovery system includes comprehensive test infrastructure to validate authentication detection capabilities.

### Test Environment Setup

The `examples/discovery/` directory contains test MCP servers for different authentication types:

#### Test Servers

1. **No Authentication Server** (`no-auth-server.py`)
   - Port: 8002
   - Auth Type: None
   - Vulnerability: High
   - Purpose: Validates detection of open MCP servers

2. **Bearer Token Server** (`bearer-auth-server.py`)
   - Port: 8001
   - Auth Type: Bearer Token
   - Vulnerability: Medium
   - Test Token: `test-bearer-token-12345`

3. **OAuth Server** (`oauth-server.py`)
   - OAuth Server Port: 8003
   - MCP Server Port: 8004
   - Auth Type: OAuth 2.1
   - Vulnerability: Low
   - Test Token: `oauth-test-token`

### Running Tests

#### Automated Testing

Run the comprehensive test suite:

```bash
cd examples/discovery
./test-discovery.sh
```

This script will:
- Start all test servers
- Run the MCP discovery service against localhost
- Verify correct authentication type detection
- Validate vulnerability scoring
- Generate a detailed test report

#### Manual Testing

Start test servers manually:

```bash
cd examples/discovery
./start-test-servers.sh
```

Or start individually:

```bash
# Terminal 1
python3 no-auth-server.py

# Terminal 2
python3 bearer-auth-server.py

# Terminal 3
python3 oauth-server.py
```

#### Docker Testing

For containerized testing:

```bash
cd examples/discovery
docker-compose up -d
```

### Manual Server Testing

Test each server individually:

```bash
# No-auth server (should work)
curl -X POST http://localhost:8002/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'

# Bearer auth server (should fail without token)
curl -X POST http://localhost:8001/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'

# Bearer auth server (should work with token)
curl -X POST http://localhost:8001/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-bearer-token-12345" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'

# OAuth server (should fail without token)
curl -X POST http://localhost:8004/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'

# OAuth server (should work with token)
curl -X POST http://localhost:8004/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer oauth-test-token" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'
```

### Discovery Testing

Test the discovery system against running servers:

```bash
# Scan localhost for test servers
curl -X POST http://localhost:8911/scan \
  -H "Content-Type: application/json" \
  -d '{
    "scanRanges": ["127.0.0.1/32"],
    "ports": [8001, 8002, 8004],
    "timeout": "10s"
  }'

# Check scan status (replace SCAN_ID with actual ID from response)
curl http://localhost:8911/scan/SCAN_ID
```

### Expected Results

When scanning `127.0.0.1/32` with ports `8001,8002,8004`, you should see:

```json
{
  "scanId": "scan-1762180330654346000",
  "status": "completed",
  "serverCount": 3,
  "results": [
    {
      "id": "mcp-127-0-0-1-8001-1762180330654346000",
      "address": "http://127.0.0.1:8001",
      "protocol": "MCP",
      "connection": "StreamableHttp",
      "status": "discovered",
      "vulnerability_score": "medium",
      "metadata": {
        "auth_type": "bearer",
        "auth_scheme": "Bearer",
        "detection_method": "http-auth-response"
      }
    },
    {
      "id": "mcp-127-0-0-1-8002-1762180330654346000",
      "address": "http://127.0.0.1:8002",
      "protocol": "MCP",
      "connection": "StreamableHttp",
      "status": "discovered",
      "vulnerability_score": "high",
      "metadata": {
        "auth_type": "none",
        "detection_method": "http-success-response"
      }
    },
    {
      "id": "mcp-127-0-0-1-8004-1762180330654346000",
      "address": "http://127.0.0.1:8004",
      "protocol": "MCP",
      "connection": "StreamableHttp",
      "status": "discovered",
      "vulnerability_score": "low",
      "metadata": {
        "auth_type": "oauth",
        "detection_method": "http-auth-response"
      }
    }
  ]
}
```

### Test Validation Checklist

- [ ] No-auth server detected with `auth_type: "none"` and `vulnerability_score: "high"`
- [ ] Bearer auth server detected with `auth_type: "bearer"` and `vulnerability_score: "medium"`
- [ ] OAuth server detected with `auth_type: "oauth"` and `vulnerability_score: "low"`
- [ ] All servers return valid MCP protocol responses
- [ ] Authentication enforcement works correctly for protected servers
- [ ] Discovery scan completes without errors
- [ ] Server metadata includes correct detection methods

### Troubleshooting Tests

#### Servers Not Starting
```bash
# Check if ports are available
netstat -tlnp | grep :800

# Kill existing processes
pkill -f "python3.*-server.py"

# Check Python dependencies
pip install fastmcp flask flask-cors
```

#### Discovery Not Finding Servers
1. Verify MCP gateway is running on port 8911
2. Check server logs for errors
3. Test manual connectivity: `curl http://localhost:8001/mcp`
4. Ensure firewall allows local connections

#### Authentication Detection Issues
- Check server logs for WWW-Authenticate headers
- Verify OAuth metadata endpoints are accessible
- Test manual authentication with provided tokens

## Future Enhancements

- **IPv6 Support**: Full IPv6 CIDR range scanning
- **Advanced Auth Detection**: Custom authentication scheme recognition
- **Service Discovery**: Integration with DNS-SD, Consul, etc.
- **Performance Optimization**: Async scanning with worker pools
- **Compliance Reporting**: Security posture assessment reports