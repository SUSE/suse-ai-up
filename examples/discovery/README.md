# MCP Discovery Test Servers

This directory contains test MCP servers for validating the discovery system's authentication detection capabilities.

## Test Servers

### 1. No Authentication Server (`no-auth-server.py`)
- **Port**: 8002
- **Auth Type**: None
- **Vulnerability**: High
- **Purpose**: Tests detection of unauthenticated MCP servers

### 2. Bearer Token Server (`bearer-auth-server.py`)
- **Port**: 8001
- **Auth Type**: Bearer Token
- **Vulnerability**: Medium
- **Test Token**: `test-bearer-token-12345`
- **Purpose**: Tests Bearer token authentication detection

### 3. OAuth Server (`oauth-server.py`)
- **OAuth Server Port**: 8003
- **MCP Server Port**: 8004
- **Auth Type**: OAuth 2.1
- **Vulnerability**: Low
- **Test Token**: `oauth-test-token`
- **Purpose**: Tests OAuth authentication detection

## Running the Test Servers

### Individual Servers

```bash
# No auth server
python3 no-auth-server.py

# Bearer auth server
python3 bearer-auth-server.py

# OAuth servers (both OAuth server and MCP server)
python3 oauth-server.py
```

### All Servers Together

```bash
# Start all test servers in background
./start-test-servers.sh

# Or manually:
python3 no-auth-server.py &
python3 bearer-auth-server.py &
python3 oauth-server.py &
```

## Testing Discovery

### Manual Testing

First, determine your host IP address:

```bash
python3 -c "
import socket
try:
    hostname = socket.gethostname()
    ip_address = socket.gethostbyname(hostname)
    if ip_address.startswith('127.'):
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.connect(('8.8.8.8', 80))
        ip_address = s.getsockname()[0]
        s.close()
    print(f'Host IP: {ip_address}')
except Exception:
    print('Host IP: 127.0.0.1')
"
```

Then test each server individually (replace `YOUR_HOST_IP` with your actual host IP):

```bash
# No-auth server (should work)
curl -X POST http://YOUR_HOST_IP:8002/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'

# Bearer auth server (should fail without token)
curl -X POST http://YOUR_HOST_IP:8001/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'

# Bearer auth server (should work with token)
curl -X POST http://YOUR_HOST_IP:8001/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-bearer-token-12345" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'

# OAuth server (should fail without token)
curl -X POST http://YOUR_HOST_IP:8004/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'

# OAuth server (should work with token)
curl -X POST http://YOUR_HOST_IP:8004/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer oauth-test-token" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'
```

### Automated Testing

Run the automated test script:

```bash
./test-discovery.sh
```

This script will:
1. Start all test servers
2. Run the MCP discovery service against localhost
3. Verify that all auth types are detected correctly
4. Check vulnerability scoring
5. Generate a test report

## Expected Discovery Results

When running discovery against `YOUR_HOST_IP/32` with ports `8001,8002,8004`, you should see (replace `YOUR_HOST_IP` with your actual host IP):

```json
{
  "servers": [
    {
      "address": "http://YOUR_HOST_IP:8001",
      "auth_type": "bearer",
      "vulnerability_score": "medium"
    },
    {
      "address": "http://YOUR_HOST_IP:8002",
      "auth_type": "none",
      "vulnerability_score": "high"
    },
    {
      "address": "http://YOUR_HOST_IP:8004",
      "auth_type": "oauth",
      "vulnerability_score": "low"
    }
  ]
}
```

## Docker Compose

For containerized testing:

```bash
docker-compose up -d
```

This starts all test servers in containers with proper networking.

## Troubleshooting

### Port Conflicts
If ports 8001-8004 are in use, modify the `PORT` environment variable:

```bash
PORT=9001 python3 bearer-auth-server.py
OAUTH_PORT=9003 MCP_PORT=9004 python3 oauth-server.py
```

### Import Errors
Ensure FastMCP is installed:

```bash
pip install fastmcp flask flask-cors
```

### Discovery Not Finding Servers
1. Verify servers are running: `netstat -tlnp | grep :800`
2. Check server logs for errors
3. Ensure MCP discovery service is running on port 8911
4. Test manual connectivity: `curl http://localhost:8001/mcp`</content>
</xai:function_call">## Creating Test Infrastructure

Now I need to create the automated testing script and docker-compose file. Let me start with the test-discovery.sh script. I'll create a todo list to track this work. 

<xai:function_call name="todowrite">
<parameter name="todos">[{"content": "Create test-discovery.sh automated testing script", "status": "in_progress", "priority": "high", "id": "create_test_script"}, {"content": "Create docker-compose.yml for containerized testing", "status": "pending", "priority": "medium", "id": "create_docker_compose"}, {"content": "Create start-test-servers.sh helper script", "status": "pending", "priority": "medium", "id": "create_start_script"}, {"content": "Add comprehensive testing section to docs/discovery.md", "status": "pending", "priority": "high", "id": "update_discovery_docs"}, {"content": "Test the complete discovery system with all auth types", "status": "pending", "priority": "high", "id": "validate_system"}]