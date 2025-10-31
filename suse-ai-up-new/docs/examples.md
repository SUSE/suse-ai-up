# SUSE AI Universal Proxy - Examples

This document provides practical examples for using the SUSE AI Universal Proxy with various MCP server configurations and connection types.

## Prerequisites

Before running these examples, ensure you have:

1. **Started the MCP example server:**
   ```bash
   cd examples && pip install -r requirements.txt
   fastmcp run src/main.py:app --transport streamable-http --port 8000 --host 0.0.0.0
   ```

2. **Started the SUSE AI Universal Proxy:**
   ```bash
   go build ./cmd/service && ./service
   ```

## Basic MCP Server Testing

### 1. Check Gateway Status
```bash
# Verify gateway is running
curl -s http://localhost:8911/adapters | jq '.'
```

### 2. Create MCP Adapter
```bash
# Create adapter pointing to local MCP server
curl -X POST http://localhost:8911/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "mcp-demo",
    "imageName": "mcp-example",
    "imageVersion": "1.0.0",
    "description": "Demo MCP server with add tool"
  }'
```

### 3. Initialize MCP Session
```bash
# Initialize MCP connection - this establishes the session
curl -X POST http://localhost:8911/adapters/mcp-demo/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {},
      "clientInfo": {
        "name": "curl-test",
        "version": "1.0"
      }
    }
  }'
```

### 4. List Available Tools
```bash
# List tools - use the session ID from the initialize response
curl -X POST http://localhost:8911/adapters/mcp-demo/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "mcp-session-id: YOUR_SESSION_ID_HERE" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/list"
  }'
```

### 5. Call the Add Tool
```bash
# Call the add tool
curl -X POST http://localhost:8911/adapters/mcp-demo/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "mcp-session-id: YOUR_SESSION_ID_HERE" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "add",
      "arguments": {
        "a": 5,
        "b": 3
      }
    }
  }'
```

### 6. Check Adapter Status
```bash
# Check adapter deployment status
curl -s http://localhost:8911/adapters/mcp-demo/status | jq '.'
```

### 7. Clean Up
```bash
# Delete the adapter
curl -X DELETE http://localhost:8911/adapters/mcp-demo
```

## LocalStdio Adapters

The proxy supports spawning local MCP servers using the `LocalStdio` connection type. This allows you to run MCP servers as local processes without needing Docker containers or Kubernetes deployments.

### Prerequisites
- Start the SUSE AI Universal Proxy:
  ```bash
  go build ./cmd/service && ./service
  ```

### 1. Create LocalStdio Adapter
```bash
# Create adapter for sequential thinking MCP server
curl -X POST http://localhost:8911/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "sequential-thinking",
    "protocol": "MCP",
    "connectionType": "LocalStdio",
    "command": "npx",
    "args": ["-y", "@modelcontextprotocol/server-sequential-thinking"],
    "environmentVariables": {},
    "description": "Sequential thinking MCP server"
  }'
```

### 2. Test the LocalStdio Adapter
```bash
# Initialize MCP connection - this spawns the local process
curl -X POST http://localhost:8911/adapters/sequential-thinking/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {},
      "clientInfo": {
        "name": "curl-test",
        "version": "1.0"
      }
    }
  }'
```

### 3. List Available Tools
```bash
# List tools - use the session ID from the initialize response
curl -X POST http://localhost:8911/adapters/sequential-thinking/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "mcp-session-id: YOUR_SESSION_ID_HERE" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/list"
  }'
```

### 4. Call Tools
```bash
# Call a tool using the session ID
curl -X POST http://localhost:8911/adapters/sequential-thinking/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "mcp-session-id: YOUR_SESSION_ID_HERE" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "sequential-thinking-tool",
      "arguments": {
        "input": "analyze this problem step by step"
      }
    }
  }'
```

### 5. Check Adapter Status
```bash
# Check local adapter status
curl -s http://localhost:8911/adapters/sequential-thinking/status | jq '.'
```

### 6. Clean Up
```bash
# Delete the local adapter
curl -X DELETE http://localhost:8911/adapters/sequential-thinking
```

### Additional LocalStdio Examples

#### Python-based MCP Server
```bash
# Create adapter for a Python MCP server
curl -X POST http://localhost:8911/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "python-mcp-server",
    "protocol": "MCP",
    "connectionType": "LocalStdio",
    "command": "python3",
    "args": ["my_mcp_server.py"],
    "environmentVariables": {
      "PYTHONPATH": "/path/to/server"
    },
    "description": "Custom Python MCP server"
  }'
```

#### Node.js MCP Server with Environment Variables
```bash
# Create adapter for Node.js MCP server with custom env vars
curl -X POST http://localhost:8911/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "nodejs-mcp-server",
    "protocol": "MCP",
    "connectionType": "LocalStdio",
    "command": "node",
    "args": ["server.js", "--port", "3000"],
    "environmentVariables": {
      "NODE_ENV": "production",
      "API_KEY": "your-api-key"
    },
    "description": "Node.js MCP server with configuration"
  }'
```

### VS Code Configuration
For VS Code integration with LocalStdio adapters:

```json
{
  "servers": {
    "sequential-thinking": {
      "url": "http://localhost:8911/adapters/sequential-thinking/mcp"
    }
  }
}
```

## Session Management Examples

The proxy includes comprehensive session management capabilities for MCP protocol sessions.

### Prerequisites
- Start the SUSE AI Universal Proxy:
  ```bash
  go build ./cmd/service && ./service
  ```
- Create an adapter (using LocalStdio or any other connection type)

### 1. List Active Sessions
```bash
# List all sessions for an adapter
curl -s http://localhost:8911/adapters/sequential-thinking/sessions | jq '.'
```

**Response:**
```json
{
  "adapterName": "sequential-thinking",
  "sessions": [
    {
      "sessionId": "session-123",
      "adapterName": "sequential-thinking",
      "targetAddress": "local-process",
      "connectionType": "LocalStdio",
      "createdAt": "2025-10-28T12:00:00Z",
      "lastActivity": "2025-10-28T12:05:00Z",
      "status": "active"
    }
  ]
}
```

### 2. Get Session Details
```bash
# Get detailed information about a specific session
curl -s http://localhost:8911/adapters/sequential-thinking/sessions/session-123 | jq '.'
```

### 3. Reinitialize Session
```bash
# Create a new session (reinitialize)
curl -X POST http://localhost:8911/adapters/sequential-thinking/sessions \
  -H "Content-Type: application/json" \
  -d '{
    "forceReinitialize": true,
    "clientInfo": {
      "name": "my-client",
      "version": "1.0"
    }
  }'
```

**Response:**
```json
{
  "sessionId": "session-456",
  "message": "Session reinitialized successfully",
  "adapterName": "sequential-thinking"
}
```

### 4. Delete a Specific Session
```bash
# Invalidate a specific session
curl -X DELETE http://localhost:8911/adapters/sequential-thinking/sessions/session-123
```

**Response:**
```json
{
  "message": "Session deleted successfully",
  "sessionId": "session-123"
}
```

### 5. Delete All Sessions for an Adapter
```bash
# Remove all sessions for an adapter
curl -X DELETE http://localhost:8911/adapters/sequential-thinking/sessions
```

**Response:**
```json
{
  "message": "All sessions deleted successfully",
  "adapterName": "sequential-thinking"
}
```

## Network Discovery Examples

This section provides examples of using the MCP server discovery functionality to automatically find and register MCP servers on your network.

### Prerequisites
- Start the MCP example server (or any MCP server):
  ```bash
  cd examples && pip install -r requirements.txt
  fastmcp run src/main.py:app --transport streamable-http --port 8000 --host 0.0.0.0
  ```
- Start the SUSE AI Universal Proxy:
  ```bash
  go build ./cmd/service && ./service
  ```

### 1. Scan for MCP Servers
```bash
# Scan localhost for MCP servers on port 8000
curl -X POST http://localhost:8911/scan \
  -H "Content-Type: application/json" \
  -d '{
    "scanRanges": ["127.0.0.1/32"],
    "ports": [8000]
  }'
```

**Response:**
```json
{
  "scanId": "scan-1761654988618691000",
  "status": "completed",
  "serverCount": 1,
  "results": [
    {
      "id": "mcp-1761654988623408000",
      "address": "http://127.0.0.1:8000",
      "protocol": "MCP",
      "connection": "SSE",
      "status": "healthy",
      "lastSeen": "2025-10-28T13:36:28.623417+01:00",
      "metadata": {
        "detectionMethod": "sse"
      }
    }
  ]
}
```

### 2. List Discovered Servers
```bash
# List all discovered MCP servers
curl http://localhost:8911/servers
```

### 3. Register Discovered Server
```bash
# Register a discovered server as an adapter
curl -X POST http://localhost:8911/register \
  -H "Content-Type: application/json" \
  -d '{
    "discoveredServerId": "mcp-1761654988623408000"
  }'
```

**Response:**
```json
{
  "message": "Server registration prepared",
  "adapterData": {
    "name": "discovered-127-0-0-1-1761654999",
    "imageName": "mcp-proxy",
    "imageVersion": "1.0.0",
    "protocol": "MCP",
    "connectionType": "SSE",
    "environmentVariables": {
      "MCP_PROXY_URL": "http://127.0.0.1:8000/mcp"
    },
    "replicaCount": 0,
    "description": "Auto-discovered MCP server at http://127.0.0.1:8000",
    "useWorkloadIdentity": false
  },
  "note": "Integration with ManagementService needed for actual adapter creation"
}
```

### 4. Create Adapter from Registration Data
```bash
# Create the adapter using the data from registration
curl -X POST http://localhost:8911/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "discovered-mcp-server",
    "imageName": "mcp-proxy",
    "imageVersion": "1.0.0",
    "protocol": "MCP",
    "connectionType": "SSE",
    "environmentVariables": {
      "MCP_PROXY_URL": "http://127.0.0.1:8000/mcp"
    },
    "description": "Auto-discovered MCP server"
  }'
```

### 5. Test the Registered Adapter
```bash
# Test SSE connection
curl -N -H "Accept: text/event-stream" http://localhost:8911/adapters/discovered-mcp-server/sse

# Test streamable HTTP connection
curl -X POST http://localhost:8911/adapters/discovered-mcp-server/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {},
      "clientInfo": {
        "name": "discovery-test",
        "version": "1.0"
      }
    }
  }'
```

## MCP Server Authentication Examples

The SUSE AI Universal Proxy supports MCP servers with built-in authentication. The included `examples` provides an example of an authenticated MCP server using Bearer token authentication.

### Authenticated MCP Server Example

The `examples` includes an authenticated version that requires Bearer token authentication:

- **Default Token**: `mcp-example-token-12345` (configurable via `MCP_AUTH_TOKEN` environment variable)
- **Authentication Method**: Bearer token in `Authorization` header
- **Required Scopes**: `read` scope for tool access

#### Running the Authenticated Server
```bash
cd examples && pip install -r requirements.txt
python3 -c "from src.main import app; app.run(transport='streamable-http', port=8001)"
```

#### Testing Authentication

**Without Authentication (should fail):**
```bash
curl -X POST http://localhost:8001/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test-client", "version": "1.0.0"}}}'
```

**Response:**
```json
{"error": "invalid_token", "error_description": "Authentication required"}
```

**With Valid Authentication:**
```bash
curl -X POST http://localhost:8001/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer mcp-example-token-12345" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test-client", "version": "1.0.0"}}}'
```

**Response:**
```json
event: message
data: {"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2024-11-05","capabilities":{"experimental":{},"prompts":{"listChanged":true},"resources":{"subscribe":false,"listChanged":true},"tools":{"listChanged":true}},"serverInfo":{"name":"MCP Example Server with Auth","version":"1.19.0"}}}
```

#### VS Code Configuration for Authenticated Servers
For authenticated MCP servers, configure VS Code with authentication headers:

```json
{
  "servers": {
    "authenticated-mcp-server": {
      "url": "http://localhost:8001/mcp",
      "headers": {
        "Authorization": "Bearer mcp-example-token-12345"
      }
    }
  }
}
```

## Expected Responses

### Initialize Response (contains session ID in headers)
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": "2024-11-05",
    "capabilities": {
      "tools": {"listChanged": true}
    },
    "serverInfo": {
      "name": "MCP Example Server",
      "version": "1.19.0"
    }
  }
}
```

### Tools List Response
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "tools": [
      {
        "name": "add",
        "description": "Add two numbers",
        "inputSchema": {
          "type": "object",
          "properties": {
            "a": {"type": "integer"},
            "b": {"type": "integer"}
          },
          "required": ["a", "b"]
        }
      }
    ]
  }
}
```

### Tool Call Response
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "result": 8
  }
}
```

## Notes

- **Session ID**: Copy the `mcp-session-id` from the initialize response headers and use it in subsequent requests
- **SSE Format**: MCP uses Server-Sent Events, so responses start with `event: message\ndata: {...}`
- **Local Mode**: The gateway runs in local development mode, proxying to `localhost:8000`
- **Process Lifecycle**: Local process stays running across sessions until adapter is deleted
- **Environment Variables**: All specified environment variables are passed to the spawned process
- **Resource Limits**: Currently no CPU/memory limits are enforced on local processes
- **Logging**: Process stdout/stderr is not captured - check the proxy logs for MCP protocol communication
- **Security**: LocalStdio allows arbitrary command execution - use appropriate security measures in production
- **Automatic Tracking**: Sessions are automatically tracked when MCP requests are made
- **Activity Updates**: Session `lastActivity` is updated on each request
- **Cleanup**: Sessions persist until explicitly deleted or adapter is removed
- **Security**: Session operations are scoped to the specific adapter
- **Monitoring**: Use session listing to monitor active connections and debug issues
- **Discovery**: Works in both local development and Kubernetes environments
- **Scanned servers**: Cached and can be listed without re-scanning
- **Registration**: Creates adapter configuration but requires manual adapter creation