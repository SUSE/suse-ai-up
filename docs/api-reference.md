# SUSE AI Universal Proxy - API Reference

This document provides comprehensive API reference for the SUSE AI Universal Proxy service endpoints.

## Base URL
```
http://localhost:8911
```

## Authentication
Most endpoints do not require authentication in development mode. For production deployments, OAuth 2.1 authentication may be required for certain operations.

## MCP Server Management (Adapters)

### Create Adapter
Deploy and register a new MCP server.

```http
POST /api/v1/adapters
Content-Type: application/json
```

**Request Body:**
```json
{
  "name": "mcp-example",
  "imageName": "mcp-example",
  "imageVersion": "1.0.0",
  "description": "Example MCP server",
  "protocol": "MCP",
  "connectionType": "LocalStdio",
  "command": "npx",
  "args": ["-y", "@modelcontextprotocol/server-sequential-thinking"],
  "environmentVariables": {},
  "replicaCount": 1
}
```

**Response (201 Created):**
```json
{
  "name": "mcp-example",
  "status": "creating",
  "createdAt": "2025-10-28T12:00:00Z"
}
```

### List Adapters
List all MCP servers the user can access.

```http
GET /api/v1/adapters
```

**Response (200 OK):**
```json
[
  {
    "name": "mcp-example",
    "status": "running",
    "description": "Example MCP server",
    "createdAt": "2025-10-28T12:00:00Z",
    "lastActivity": "2025-10-28T12:05:00Z"
  }
]
```

### Get Adapter Details
Retrieve metadata for a specific adapter.

```http
GET /api/v1/adapters/{name}
```

**Parameters:**
- `name` (path): Adapter name

**Response (200 OK):**
```json
{
  "name": "mcp-example",
  "status": "running",
  "description": "Example MCP server",
  "protocol": "MCP",
  "connectionType": "LocalStdio",
  "command": "npx",
  "args": ["-y", "@modelcontextprotocol/server-sequential-thinking"],
  "environmentVariables": {},
  "createdAt": "2025-10-28T12:00:00Z",
  "lastActivity": "2025-10-28T12:05:00Z"
}
```

### Get Adapter Status
Check the deployment status of a specific adapter.

```http
GET /api/v1/adapters/{name}/status
```

**Parameters:**
- `name` (path): Adapter name

**Response (200 OK):**
```json
{
  "name": "mcp-example",
  "status": "running",
  "phase": "Running",
  "message": "Adapter is healthy",
  "lastCheck": "2025-10-28T12:05:00Z"
}
```

### Get Adapter Logs
Access the running logs of a specific adapter.

```http
GET /api/v1/adapters/{name}/logs
```

**Parameters:**
- `name` (path): Adapter name
- `tail` (query, optional): Number of lines to return from the end (default: 100)
- `since` (query, optional): RFC3339 timestamp to start logs from

**Response (200 OK):**
```
2025-10-28T12:00:00Z INFO Starting MCP server
2025-10-28T12:00:01Z INFO Server listening on stdio
2025-10-28T12:05:00Z INFO Processed initialize request
```

### Update Adapter
Update the deployment configuration of an adapter.

```http
PUT /api/v1/adapters/{name}
Content-Type: application/json
```

**Parameters:**
- `name` (path): Adapter name

**Request Body:**
```json
{
  "environmentVariables": {
    "DEBUG": "true"
  },
  "replicaCount": 2
}
```

**Response (200 OK):**
```json
{
  "name": "mcp-example",
  "status": "updating",
  "message": "Adapter update initiated"
}
```

### Delete Adapter
Remove a specific adapter and clean up its resources.

```http
DELETE /api/v1/adapters/{name}
```

**Parameters:**
- `name` (path): Adapter name

**Response (204 No Content):**

## MCP Communication

### Establish SSE Connection
Establish an initial Server-Sent Events connection for MCP communication.

```http
GET /api/v1/adapters/{name}/sse
Accept: text/event-stream
```

**Parameters:**
- `name` (path): Adapter name

**Response (200 OK):**
```
event: message
data: {"jsonrpc":"2.0","id":1,"result":{...}}

event: message
data: {"jsonrpc":"2.0","id":2,"result":{...}}
```

### Send MCP Messages
Send subsequent requests using session_id for established sessions.

```http
POST /api/v1/adapters/{name}/messages
Content-Type: application/json
mcp-session-id: {sessionId}
```

**Parameters:**
- `name` (path): Adapter name
- `sessionId` (header): MCP session identifier

**Request Body:**
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/list",
  "params": {}
}
```

**Response (200 OK):**
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

### Establish Streamable HTTP Connection
Establish a streamable HTTP connection for MCP protocol communication.

```http
POST /api/v1/adapters/{name}/mcp
Content-Type: application/json
Accept: application/json, text/event-stream
```

**Parameters:**
- `name` (path): Adapter name

**Request Body (Initialize):**
```json
{
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
}
```

**Response (200 OK):**
```
event: message
data: {"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2024-11-05","capabilities":{...},"serverInfo":{...}}}
```

## Session Management

### List Sessions
List all active sessions for an adapter.

```http
GET /api/v1/adapters/{name}/sessions
```

**Parameters:**
- `name` (path): Adapter name

**Response (200 OK):**
```json
{
  "adapterName": "mcp-example",
  "sessions": [
    {
      "sessionId": "session-123",
      "adapterName": "mcp-example",
      "targetAddress": "local-process",
      "connectionType": "LocalStdio",
      "createdAt": "2025-10-28T12:00:00Z",
      "lastActivity": "2025-10-28T12:05:00Z",
      "status": "active"
    }
  ]
}
```

### Get Session Details
Get detailed information about a specific session.

```http
GET /api/v1/adapters/{name}/sessions/{sessionId}
```

**Parameters:**
- `name` (path): Adapter name
- `sessionId` (path): Session identifier

**Response (200 OK):**
```json
{
  "sessionId": "session-123",
  "adapterName": "mcp-example",
  "targetAddress": "local-process",
  "connectionType": "LocalStdio",
  "createdAt": "2025-10-28T12:00:00Z",
  "lastActivity": "2025-10-28T12:05:00Z",
  "status": "active",
  "metadata": {
    "protocolVersion": "2024-11-05",
    "clientInfo": {
      "name": "test-client",
      "version": "1.0.0"
    }
  }
}
```

### Create/Reinitialize Session
Create a new session or reinitialize an existing one for an adapter.

```http
POST /api/v1/adapters/{name}/sessions
Content-Type: application/json
```

**Parameters:**
- `name` (path): Adapter name

**Request Body:**
```json
{
  "forceReinitialize": true,
  "clientInfo": {
    "name": "my-client",
    "version": "1.0"
  }
}
```

**Response (201 Created):**
```json
{
  "sessionId": "session-456",
  "message": "Session reinitialized successfully",
  "adapterName": "mcp-example"
}
```

### Delete Specific Session
Invalidate and remove a specific session.

```http
DELETE /api/v1/adapters/{name}/sessions/{sessionId}
```

**Parameters:**
- `name` (path): Adapter name
- `sessionId` (path): Session identifier

**Response (200 OK):**
```json
{
  "message": "Session deleted successfully",
  "sessionId": "session-123"
}
```

### Delete All Sessions
Remove all sessions for an adapter.

```http
DELETE /api/v1/adapters/{name}/sessions
```

**Parameters:**
- `name` (path): Adapter name

**Response (200 OK):**
```json
{
  "message": "All sessions deleted successfully",
  "adapterName": "mcp-example"
}
```

## Network Discovery

### Scan for MCP Servers
Perform network scanning to discover MCP servers on the network.

```http
POST /api/v1/discovery/scan
```

**Response (200 OK):**
```json
{
  "count": 1,
  "discovered": [
    {
      "id": "mcp-192.168.1.74-8002--mcp",
      "name": "MCP Example Server (No Auth)",
      "address": "http://192.168.1.74:8002",
      "protocol": "MCP",
      "connection": "SSE",
      "status": "discovered",
      "vulnerability_score": "high",
      "metadata": {
        "auth_type": "none",
        "endpoint": "/mcp",
        "port": "8002",
        "server_name": "MCP Example Server (No Auth)",
        "vulnerability_score": "high"
      }
    }
  ],
  "errors": null
}
```

### List Discovered Servers
Get all discovered MCP servers from previous scans.

```http
GET /api/v1/discovery/servers
```

**Response (200 OK):**
```json
{
  "count": 1,
  "servers": [
    {
      "id": "mcp-192.168.1.74-8002--mcp",
      "name": "MCP Example Server (No Auth)",
      "address": "http://192.168.1.74:8002",
      "protocol": "MCP",
      "connection": "SSE",
      "status": "discovered",
      "vulnerability_score": "high"
    }
  ]
}
```

### Get Discovered Server
Get details of a specific discovered server by ID.

```http
GET /api/v1/discovery/servers/{id}
```

**Parameters:**
- `id` (path): Discovered server ID

**Response (200 OK):**
```json
{
  "id": "mcp-192.168.1.74-8002--mcp",
  "name": "MCP Example Server (No Auth)",
  "address": "http://192.168.1.74:8002",
  "protocol": "MCP",
  "connection": "SSE",
  "status": "discovered",
  "lastSeen": "2025-11-06T14:27:44.929581+01:00",
  "vulnerability_score": "high",
  "metadata": {
    "auth_type": "none",
    "endpoint": "/mcp",
    "port": "8002",
    "server_name": "MCP Example Server (No Auth)",
    "vulnerability_score": "high"
  }
}
```

**Response (404 Not Found):**
```json
{
  "error": "Server not found"
}
```
  "scanId": "scan-1761654988618691000",
  "status": "completed",
  "serverCount": 2,
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

### Register Discovered Server
Register a discovered server as an adapter.

```http
POST /api/v1/register
Content-Type: application/json
```

**Request Body:**
```json
{
  "discoveredServerId": "mcp-1761654988623408000"
}
```

**Response (200 OK):**
```json
{
  "message": "Adapter created successfully",
  "adapter": {
    "id": "discovered-127-0-0-1-8000",
    "name": "discovered-127-0-0-1-8000",
    "imageName": "mcp-proxy",
    "imageVersion": "1.0.0",
    "protocol": "MCP",
    "connectionType": "RemoteHttp",
    "environmentVariables": {
      "MCP_PROXY_URL": "http://127.0.0.1:8000",
      "MCP_SERVER_AUTH_TYPE": "none"
    },
    "replicaCount": 1,
    "description": "Auto-discovered MCP server at http://127.0.0.1:8000 [AUTO-SECURED]",
    "useWorkloadIdentity": false,
    "remoteUrl": "http://127.0.0.1:8000",
    "authentication": {
      "required": true,
      "type": "bearer",
      "bearerToken": {
        "token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
        "dynamic": true,
        "expiresAt": "2025-01-07T12:00:00Z"
      }
    },
    "createdBy": "system",
    "createdAt": "2025-01-06T12:00:00Z",
    "lastUpdatedAt": "2025-01-06T12:00:00Z"
  },
  "security_note": "High-risk server automatically secured with bearer token authentication. Original server had no authentication.",
  "token_info": {
    "tokenId": "token-1736169600",
    "accessToken": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
    "tokenType": "Bearer",
    "expiresAt": "2025-01-07T12:00:00Z",
    "issuedAt": "2025-01-06T12:00:00Z",
    "audience": "http://localhost:8911/adapters/discovered-127-0-0-1-8000",
    "issuer": "suse-ai-up",
    "subject": "adapter-discovered-127-0-0-1-8000",
    "scope": "mcp:read mcp:write server:mcp-127-0-0-1-8000-1234567890 risk:high"
  }
}
```

**Security Behavior:**

- **High vulnerability servers** (`vulnerability_score: "high"`): Automatically secured with bearer token authentication
- **Medium vulnerability servers** (`vulnerability_score: "medium"`): Optional authentication configured
- **Low vulnerability servers** (`vulnerability_score: "low"`): No additional authentication needed

**Error Responses:**

**400 Bad Request:**
```json
{
  "error": "Invalid request body",
  "details": "discoveredServerId is required"
}
```

**404 Not Found:**
```json
{
  "error": "Discovered server not found",
  "details": "Server ID 'mcp-invalid-id' not found in discovery results"
}
```

**500 Internal Server Error:**
```json
{
  "error": "Failed to create adapter",
  "details": "Failed to configure authentication: token generation failed"
}
```

## Plugin Service Management

### List Registered Services
List all registered plugin services.

```http
GET /api/v1/plugins/services
```

**Response (200 OK):**
```json
{
  "services": [
    {
      "service_id": "smartagents-main",
      "service_type": "smartagents",
      "service_url": "http://localhost:8910",
      "version": "1.0.0",
      "status": "healthy",
      "capabilities": [
        {
          "path": "/v1/*",
          "methods": ["GET", "POST"],
          "description": "Smart Agents API endpoints"
        }
      ],
      "registered_at": "2025-10-28T12:00:00Z",
      "last_health_check": "2025-10-28T12:05:00Z"
    }
  ]
}
```

### Register Service
Register a new plugin service manually.

```http
POST /api/v1/plugins/register
Content-Type: application/json
```

**Request Body:**
```json
{
  "service_id": "my-service",
  "service_type": "smartagents",
  "service_url": "http://localhost:8080",
  "version": "1.0.0",
  "capabilities": [
    {
      "path": "/api/v1/*",
      "methods": ["GET", "POST"],
      "description": "My API endpoints"
    }
  ]
}
```

**Response (201 Created):**
```json
{
  "message": "Service registered successfully",
  "service_id": "my-service"
}
```

### Get Service Health
Check the health status of a specific service.

```http
GET /api/v1/plugins/services/{serviceId}/health
```

**Parameters:**
- `serviceId` (path): Service identifier

**Response (200 OK):**
```json
{
  "service_id": "smartagents-main",
  "status": "healthy",
  "message": "Service is responding normally",
  "timestamp": "2025-10-28T12:05:00Z",
  "version": "1.0.0",
  "uptime": "5m30s"
}
```

### Unregister Service
Remove a registered service.

```http
DELETE /api/v1/plugins/services/{serviceId}
```

**Parameters:**
- `serviceId` (path): Service identifier

**Response (204 No Content):**

## Error Responses

### Common Error Codes

- **400 Bad Request**: Invalid request parameters or body
- **401 Unauthorized**: Authentication required
- **403 Forbidden**: Insufficient permissions
- **404 Not Found**: Resource not found
- **409 Conflict**: Resource already exists
- **422 Unprocessable Entity**: Validation failed
- **500 Internal Server Error**: Server error
- **503 Service Unavailable**: Service temporarily unavailable

### Error Response Format
```json
{
  "error": "ErrorType",
  "message": "Human-readable error message",
  "details": {
    "field": "specific field that caused the error",
    "reason": "detailed explanation"
  },
  "timestamp": "2025-10-28T12:00:00Z"
}
```

## Rate Limiting

API endpoints are rate limited to prevent abuse. Rate limits vary by endpoint:

- Management endpoints: 100 requests per minute
- Communication endpoints: 1000 requests per minute
- Health check endpoints: Unlimited

Rate limit headers are included in responses:
```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 99
X-RateLimit-Reset: 1640995200
```

## WebSocket Support

For real-time communication, WebSocket connections are supported on certain endpoints:

```javascript
const ws = new WebSocket('ws://localhost:8911/adapters/{name}/ws');

// Send MCP message
ws.send(JSON.stringify({
  jsonrpc: "2.0",
  id: 1,
  method: "initialize",
  params: { ... }
}));

// Receive responses
ws.onmessage = (event) => {
  const response = JSON.parse(event.data);
  console.log('Received:', response);
};
```

## OpenAPI Specification

Complete API documentation is available via Swagger UI at `/docs` and as OpenAPI JSON at `/swagger/doc.json`.