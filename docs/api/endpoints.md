# SUSE AI Uniproxy API Endpoints

## Base URL
```
http://localhost:8911/api/v1
```

## Health & Monitoring

### GET /health
Health check endpoint.

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2024-01-01T12:00:00Z",
  "version": "1.0.0"
}
```

### GET /docs/*
Swagger/OpenAPI documentation.

### GET /swagger/doc.json
OpenAPI specification in JSON format.

## Discovery Endpoints

### POST /discovery/scan
Initiate network scan for MCP servers.

**Request Body:**
```json
{
  "scanRanges": ["192.168.1.0/24"],
  "ports": ["8000", "8001", "9000"],
  "timeout": "30s",
  "maxConcurrent": 10
}
```

**Response:**
```json
{
  "jobId": "scan-12345",
  "status": "running",
  "message": "Scan initiated successfully"
}
```

### GET /discovery/scan
List all scan jobs.

**Response:**
```json
[
  {
    "jobId": "scan-12345",
    "status": "completed",
    "startTime": "2024-01-01T12:00:00Z",
    "endTime": "2024-01-01T12:05:00Z",
    "serversFound": 5
  }
]
```

### GET /discovery/scan/{jobId}
Get details of a specific scan job.

**Response:**
```json
{
  "jobId": "scan-12345",
  "status": "completed",
  "startTime": "2024-01-01T12:00:00Z",
  "endTime": "2024-01-01T12:05:00Z",
  "serversFound": 5,
  "results": [...]
}
```

### DELETE /discovery/scan/{jobId}
Cancel a running scan job.

### GET /discovery/servers
List all discovered MCP servers.

### GET /discovery/servers/{id}
Get details of a specific discovered server.

## Adapter Management

### GET /adapters
List all adapters.

**Response:**
```json
[
  {
    "id": "adapter-123",
    "name": "my-mcp-server",
    "connectionType": "RemoteHttp",
    "status": "active",
    "createdAt": "2024-01-01T12:00:00Z"
  }
]
```

### POST /adapters
Create a new adapter.

**Request Body:**
```json
{
  "name": "my-mcp-server",
  "connectionType": "RemoteHttp",
  "apiBaseUrl": "https://api.example.com/mcp",
  "authentication": {
    "required": true,
    "type": "bearer",
    "bearerToken": {
      "token": "your-token-here"
    }
  },
  "mcpClientConfig": {
    "mcpServers": {
      "server1": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-everything"],
        "env": {
          "API_KEY": "your-key"
        }
      }
    }
  }
}
```

### GET /adapters/{name}
Get adapter details.

### PUT /adapters/{name}
Update an adapter.

### DELETE /adapters/{name}
Delete an adapter.

### GET /adapters/{name}/token
Get adapter authentication token.

### POST /adapters/{name}/token/validate
Validate token.

### POST /adapters/{name}/token/refresh
Refresh token.

### GET /adapters/{name}/client-token
Get client token for MCP communication.

### POST /adapters/{name}/validate-auth
Validate authentication configuration.

### POST /adapters/{name}/test-auth
Test authentication connection.

### GET /adapters/{name}/logs
Get adapter logs.

### GET /adapters/{name}/status
Get adapter status.

**Response:**
```json
{
  "readyReplicas": 1,
  "updatedReplicas": 1,
  "availableReplicas": 1,
  "image": "nginx:latest",
  "replicaStatus": "Healthy"
}
```

## Session Management

### GET /adapters/{name}/sessions
List sessions for an adapter.

### POST /adapters/{name}/sessions
Create new session.

### DELETE /adapters/{name}/sessions
Delete all sessions.

### GET /adapters/{name}/sessions/{sessionId}
Get session details.

### DELETE /adapters/{name}/sessions/{sessionId}
Delete specific session.

## MCP Protocol Endpoints

### ANY /adapters/{name}/mcp
Main MCP proxy endpoint. Accepts JSON-RPC 2.0 requests.

**Headers:**
- `Content-Type: application/json`
- `Authorization: Bearer <token>` (if required)

### GET /adapters/{name}/tools
List available tools (REST-style).

### POST /adapters/{name}/tools/{toolName}/call
Call a specific tool (REST-style).

**Request Body:**
```json
{
  "query": "latest news",
  "limit": 10
}
```

### GET /adapters/{name}/resources
List available resources (REST-style).

### GET /adapters/{name}/resources/{uri}
Read a specific resource (REST-style).

### GET /adapters/{name}/prompts
List available prompts (REST-style).

### GET /adapters/{name}/prompts/{promptName}
Get a specific prompt (REST-style).

**Query Parameters:**
- `arg1=value1`
- `arg2=value2`

## Registry Management

### GET /registry
Browse registry servers.

### GET /registry/public
List public registry servers.

### POST /registry/sync/official
Sync with official MCP server registry.

### POST /registry/upload
Upload registry entry.

### POST /registry/upload/bulk
Upload multiple registry entries.

### POST /registry/upload/local-mcp
Upload local MCP server.

### GET /registry/browse
Browse registry with filtering.

### GET /registry/{id}
Get specific MCP server details.

### PUT /registry/{id}
Update MCP server.

### DELETE /registry/{id}
Delete MCP server.

## Plugin Management

### POST /plugins/register
Register a service.

### DELETE /plugins/register/{serviceId}
Unregister a service.

### GET /plugins/services
List all services.

### GET /plugins/services/{serviceId}
Get service details.

### GET /plugins/services/type/{serviceType}
List services by type.

### GET /plugins/services/{serviceId}/health
Get service health status.

## Monitoring Endpoints

### GET /api/v1/monitoring/metrics
Get Prometheus metrics.

### GET /api/v1/monitoring/logs
Get application logs.

### GET /api/v1/monitoring/cache
Get cache statistics.

## Authentication Methods

### Bearer Token
```bash
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

### Basic Auth
```bash
Authorization: Basic dXNlcjpwYXNzd29yZA==
```

### API Key
```bash
# Header
X-API-Key: your-api-key

# Query parameter
?api_key=your-api-key

# Cookie
Cookie: api_key=your-api-key
```

## Error Responses

All endpoints return consistent error responses:

```json
{
  "error": "Error message",
  "code": "ERROR_CODE",
  "details": {...}
}
```

### Common Error Codes
- `400` - Bad Request (invalid parameters)
- `401` - Unauthorized (authentication required)
- `403` - Forbidden (insufficient permissions)
- `404` - Not Found (resource doesn't exist)
- `500` - Internal Server Error (server error)

## Rate Limiting

API endpoints are rate limited. Check response headers:
- `X-RateLimit-Limit`: Maximum requests per window
- `X-RateLimit-Remaining`: Remaining requests
- `X-RateLimit-Reset`: Time when limit resets

## Connection Types

### LocalStdio
For local MCP servers running via stdio:
```json
{
  "connectionType": "LocalStdio",
  "mcpClientConfig": {
    "mcpServers": {
      "server1": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
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
  "apiBaseUrl": "https://api.example.com/mcp"
}
```

### StreamableHttp
For servers supporting streaming responses:
```json
{
  "connectionType": "StreamableHttp",
  "apiBaseUrl": "https://api.example.com/mcp"
}
```

### SSE
For Server-Sent Events connections:
```json
{
  "connectionType": "SSE",
  "apiBaseUrl": "https://api.example.com/mcp"
}
```