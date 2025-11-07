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

For VS Code integration with the SUSE AI Universal Proxy, configure your MCP settings to use the proxy endpoints.

#### Basic VS Code Configuration (No Authentication)

```json
{
  "servers": {
    "sequential-thinking": {
      "url": "http://localhost:8911/adapters/sequential-thinking/mcp"
    },
    "filesystem": {
      "url": "http://localhost:8911/adapters/filesystem/mcp"
    }
  }
}
```

#### VS Code Configuration with Bearer Token Authentication

```json
{
  "servers": {
    "authenticated-sequential-thinking": {
      "url": "http://localhost:8911/adapters/sequential-thinking/mcp",
      "headers": {
        "Authorization": "Bearer my-secure-token-123"
      }
    },
    "authenticated-filesystem": {
      "url": "http://localhost:8911/adapters/filesystem/mcp",
      "headers": {
        "Authorization": "Bearer filesystem-token-456"
      }
    }
  }
}
```

#### VS Code Configuration with OAuth 2.1

For OAuth-protected adapters, VS Code will need to handle the OAuth flow. Configure with the OAuth endpoints:

```json
{
  "servers": {
    "oauth-mcp": {
      "url": "http://localhost:8911/adapters/oauth-mcp/mcp",
      "oauth": {
        "authorizationEndpoint": "http://localhost:8911/adapters/oauth-mcp/auth/authorize",
        "tokenEndpoint": "http://localhost:8911/adapters/oauth-mcp/auth/token",
        "clientId": "vscode-client",
        "scopes": ["read", "write"]
      }
    }
  }
}
```

#### Advanced VS Code Configuration with Multiple Authentication Types

```json
{
  "servers": {
    "local-tools": {
      "url": "http://localhost:8911/adapters/sequential-thinking/mcp",
      "description": "Local sequential thinking tools (Bearer auth)"
    },
    "file-operations": {
      "url": "http://localhost:8911/adapters/filesystem/mcp",
      "headers": {
        "Authorization": "Bearer filesystem-token-456"
      },
      "description": "File system operations with directory restrictions"
    },
    "database-access": {
      "url": "http://localhost:8911/adapters/sqlite-db/mcp",
      "headers": {
        "Authorization": "Bearer sqlite-token-101"
      },
      "description": "SQLite database operations"
    },
    "github-integration": {
      "url": "http://localhost:8911/adapters/github-api/mcp",
      "headers": {
        "Authorization": "Bearer github-token-202"
      },
      "description": "GitHub API integration"
    },
    "discovered-server": {
      "url": "http://localhost:8911/adapters/discovered-mcp-server/mcp",
      "description": "Auto-discovered MCP server"
    }
  },
  "globalSettings": {
    "timeout": 30000,
    "retryAttempts": 3,
    "enableCaching": true
  }
}
```

#### VS Code Extension Settings

Add these settings to your VS Code `settings.json` for optimal MCP proxy integration:

```json
{
  "mcp.serverTimeout": 30000,
  "mcp.enableAutoReconnect": true,
  "mcp.logLevel": "info",
  "mcp.cacheEnabled": true,
  "mcp.sessionPersistence": true
}
```

#### Testing VS Code Integration

1. **Start the SUSE AI Universal Proxy:**
   ```bash
   go build ./cmd/service && ./service
   ```

2. **Create authenticated adapters:**
   ```bash
   # Create sequential thinking adapter
   curl -X POST http://localhost:8911/adapters \
     -H "Content-Type: application/json" \
     -d '{
       "name": "sequential-thinking",
       "connectionType": "LocalStdio",
       "mcpClientConfig": {
         "mcpServers": {
           "sequential-thinking": {
             "command": "npx",
             "args": ["-y", "@modelcontextprotocol/server-sequential-thinking"]
           }
         }
       },
       "authentication": {
         "required": true,
         "type": "bearer",
         "token": "my-secure-token-123"
       }
     }'
   ```

3. **Configure VS Code with the JSON above**

4. **Test the connection in VS Code:**
   - Open Command Palette (Ctrl+Shift+P)
   - Search for "MCP: Test Connection"
   - Select your configured server
   - Verify the connection status

#### Troubleshooting VS Code Integration

**Common Issues:**

1. **Connection Refused:**
   - Ensure the proxy is running on port 8911
   - Check firewall settings

2. **Authentication Errors:**
   - Verify tokens match exactly
   - Check token expiration
   - Ensure proper header formatting

3. **Session Issues:**
   - Enable session persistence in VS Code settings
   - Check proxy logs for session errors

4. **Performance Issues:**
   - Enable caching in VS Code settings
   - Monitor proxy performance with `/api/v1/monitoring/metrics`

**Debug Mode:**

Enable debug logging in VS Code:

```json
{
  "mcp.logLevel": "debug",
  "mcp.enableTracing": true
}
```

And check proxy logs:

```bash
curl -s "http://localhost:8911/api/v1/monitoring/logs?level=debug&limit=50" | jq '.'
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

## Monitoring and Observability Examples

The SUSE AI Universal Proxy provides comprehensive monitoring endpoints to track performance, health, and system metrics.

### Prerequisites
- Start the SUSE AI Universal Proxy:
  ```bash
  go build ./cmd/service && ./service
  ```

### 1. System Performance Metrics
```bash
# Get comprehensive performance metrics
curl -s http://localhost:8911/api/v1/monitoring/metrics | jq '.'

# Response includes:
{
  "uptime": "2h15m30s",
  "totalRequests": 1247,
  "activeConnections": 8,
  "memoryUsage": {
    "allocated": "45.2MB",
    "system": "128.5MB",
    "gcCycles": 23
  },
  "requestMetrics": {
    "averageLatency": "23ms",
    "p95Latency": "145ms",
    "p99Latency": "289ms",
    "errorRate": 0.012
  },
  "adapterMetrics": {
    "totalAdapters": 5,
    "activeAdapters": 3,
    "failedAdapters": 0
  }
}
```

### 2. Recent Log Entries
```bash
# Get recent log entries with filtering
curl -s "http://localhost:8911/api/v1/monitoring/logs?level=error&limit=10" | jq '.'

# Response includes:
{
  "logs": [
    {
      "timestamp": "2025-11-07T14:23:15Z",
      "level": "error",
      "message": "Failed to connect to MCP server",
      "adapter": "problematic-adapter",
      "details": {
        "error": "connection timeout",
        "target": "http://localhost:8005"
      }
    }
  ],
  "total": 1,
  "hasMore": false
}

# Get all logs with pagination
curl -s "http://localhost:8911/api/v1/monitoring/logs?limit=50&offset=0" | jq '.'
```

### 3. Cache Statistics and Performance
```bash
# Get detailed cache statistics
curl -s http://localhost:8911/api/v1/monitoring/cache | jq '.'

# Response includes:
{
  "cacheStats": {
    "mcpCache": {
      "hitRate": 0.78,
      "totalHits": 892,
      "totalMisses": 251,
      "evictions": 12,
      "currentSize": 156,
      "maxSize": 1000
    },
    "capabilityCache": {
      "hitRate": 0.92,
      "totalHits": 1456,
      "totalMisses": 127,
      "evictions": 3,
      "currentSize": 89,
      "maxSize": 500
    }
  },
  "performanceImpact": {
    "averageResponseTimeReduction": "45%",
    "cacheEfficiency": "high"
  }
}
```

### 4. Real-time Monitoring Dashboard
```bash
# Create a simple monitoring dashboard
watch -n 5 'curl -s http://localhost:8911/api/v1/monitoring/metrics | jq ".uptime, .totalRequests, .activeConnections, .requestMetrics.averageLatency"'

# Monitor cache performance in real-time
watch -n 10 'curl -s http://localhost:8911/api/v1/monitoring/cache | jq ".cacheStats.mcpCache.hitRate, .cacheStats.capabilityCache.hitRate"'
```

### 5. Health Check and Status
```bash
# Comprehensive health check
curl -s http://localhost:8911/api/v1/monitoring/health | jq '.'

# Response includes:
{
  "status": "healthy",
  "timestamp": "2025-11-07T14:25:00Z",
  "checks": {
    "database": "healthy",
    "memory": "healthy",
    "disk": "healthy",
    "network": "healthy"
  },
  "version": "1.0.0",
  "build": "2025-11-07T12:00:00Z"
}
```

### 6. Adapter-Specific Monitoring
```bash
# Monitor specific adapter performance
curl -s http://localhost:8911/api/v1/monitoring/adapters/sequential-thinking | jq '.'

# Response includes:
{
  "adapterName": "sequential-thinking",
  "status": "active",
  "sessionCount": 3,
  "totalRequests": 234,
  "averageLatency": "18ms",
  "errorRate": 0.008,
  "lastActivity": "2025-11-07T14:24:45Z",
  "uptime": "1h45m20s"
}
```

## Enhanced Caching Examples

The proxy implements intelligent caching to improve performance and reduce redundant operations.

### 1. MCP Response Caching
```bash
# First request - cache miss (slower)
time curl -X POST http://localhost:8911/adapters/sequential-thinking/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer my-secure-token-123" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}'

# Second request - cache hit (faster)
time curl -X POST http://localhost:8911/adapters/sequential-thinking/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer my-secure-token-123" \
  -d '{"jsonrpc": "2.0", "id": 2, "method": "tools/list"}'

# Check cache impact
curl -s http://localhost:8911/api/v1/monitoring/cache | jq '.cacheStats.mcpCache'
```

### 2. Capability Caching
```bash
# Cache tool capabilities to avoid repeated discovery
curl -X POST http://localhost:8911/adapters/filesystem/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer filesystem-token-456" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}'

# Subsequent capability requests use cache
curl -s http://localhost:8911/api/v1/monitoring/cache | jq '.cacheStats.capabilityCache'
```

### 3. Cache Performance Comparison
```bash
# Clear cache (if needed)
curl -X DELETE http://localhost:8911/api/v1/monitoring/cache/clear

# Measure performance with cold cache
echo "Testing cold cache performance..."
time (for i in {1..10}; do
  curl -s -X POST http://localhost:8911/adapters/sequential-thinking/mcp \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer my-secure-token-123" \
    -d '{"jsonrpc": "2.0", "id": '$i', "method": "tools/list"}' > /dev/null
done)

# Measure performance with warm cache
echo "Testing warm cache performance..."
time (for i in {11..20}; do
  curl -s -X POST http://localhost:8911/adapters/sequential-thinking/mcp \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer my-secure-token-123" \
    -d '{"jsonrpc": "2.0", "id": '$i', "method": "tools/list"}' > /dev/null
done)
```

## Error Handling and Troubleshooting Examples

The proxy provides comprehensive error handling with detailed categorization and recovery suggestions.

### 1. Authentication Errors
```bash
# Missing authentication (401)
curl -X POST http://localhost:8911/adapters/sequential-thinking/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test", "version": "1.0"}}}'

# Response:
{
  "error": {
    "code": 401,
    "message": "Authentication required",
    "category": "authentication",
    "details": {
      "adapter": "sequential-thinking",
      "authType": "bearer",
      "suggestion": "Provide valid Authorization header with Bearer token"
    }
  }
}

# Invalid token (403)
curl -X POST http://localhost:8911/adapters/sequential-thinking/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer invalid-token" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test", "version": "1.0"}}}'

# Response:
{
  "error": {
    "code": 403,
    "message": "Invalid authentication token",
    "category": "authorization",
    "details": {
      "adapter": "sequential-thinking",
      "reason": "token_validation_failed",
      "suggestion": "Check token validity and expiration"
    }
  }
}
```

### 2. Connection and Network Errors
```bash
# Adapter not found (404)
curl -X POST http://localhost:8911/adapters/nonexistent/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize"}'

# Response:
{
  "error": {
    "code": 404,
    "message": "Adapter not found",
    "category": "resource",
    "details": {
      "adapter": "nonexistent",
      "availableAdapters": ["sequential-thinking", "filesystem", "sqlite-db"],
      "suggestion": "Check adapter name or create new adapter"
    }
  }
}

# MCP server connection failure
curl -X POST http://localhost:8911/adapters/offline-server/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize"}'

# Response:
{
  "error": {
    "code": 503,
    "message": "MCP server unavailable",
    "category": "connection",
    "details": {
      "adapter": "offline-server",
      "target": "http://localhost:9999",
      "error": "connection refused",
      "suggestion": "Verify MCP server is running and accessible"
    }
  }
}
```

### 3. Protocol and Validation Errors
```bash
# Invalid JSON-RPC request
curl -X POST http://localhost:8911/adapters/sequential-thinking/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer my-secure-token-123" \
  -d '{"invalid": "request"}'

# Response:
{
  "error": {
    "code": 400,
    "message": "Invalid JSON-RPC request",
    "category": "protocol",
    "details": {
      "reason": "missing_required_fields",
      "required": ["jsonrpc", "id", "method"],
      "suggestion": "Ensure request follows JSON-RPC 2.0 specification"
    }
  }
}

# Invalid method name
curl -X POST http://localhost:8911/adapters/sequential-thinking/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer my-secure-token-123" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "invalid_method"}'

# Response:
{
  "error": {
    "code": -32601,
    "message": "Method not found",
    "category": "protocol",
    "details": {
      "method": "invalid_method",
      "availableMethods": ["initialize", "tools/list", "tools/call"],
      "suggestion": "Use valid MCP method name"
    }
  }
}
```

### 4. Session Management Errors
```bash
# Invalid session ID
curl -X POST http://localhost:8911/adapters/sequential-thinking/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer my-secure-token-123" \
  -H "mcp-session-id: invalid-session" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}'

# Response:
{
  "error": {
    "code": 401,
    "message": "Invalid session",
    "category": "session",
    "details": {
      "sessionId": "invalid-session",
      "adapter": "sequential-thinking",
      "suggestion": "Initialize session first or use valid session ID"
    }
  }
}

# Session expired
curl -X POST http://localhost:8911/adapters/sequential-thinking/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer my-secure-token-123" \
  -H "mcp-session-id: expired-session-123" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}'

# Response:
{
  "error": {
    "code": 401,
    "message": "Session expired",
    "category": "session",
    "details": {
      "sessionId": "expired-session-123",
      "expiredAt": "2025-11-07T13:30:00Z",
      "suggestion": "Reinitialize session to continue"
    }
  }
}
```

### 5. Error Recovery Examples
```bash
# Recover from authentication error by providing proper token
curl -X POST http://localhost:8911/adapters/sequential-thinking/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer my-secure-token-123" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "recovery-test", "version": "1.0"}}}'

# Recover from session error by reinitializing
curl -X POST http://localhost:8911/adapters/sequential-thinking/sessions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer my-secure-token-123" \
  -d '{"forceReinitialize": true, "clientInfo": {"name": "recovery-client", "version": "1.0"}}'

# Check system health after errors
curl -s http://localhost:8911/api/v1/monitoring/health | jq '.'
```