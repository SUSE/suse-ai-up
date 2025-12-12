# SUSE AI Uniproxy API Documentation

## Overview

The SUSE AI Uniproxy API provides a comprehensive REST interface for managing MCP (Model Context Protocol) servers, adapters, and integrations. The API supports multiple transport protocols including HTTP, SSE (Server-Sent Events), and WebSocket for real-time communication.

### Base URL
```
http://localhost:8911/api/v1
```

### Authentication
The API supports multiple authentication methods:
- **Bearer Token**: `Authorization: Bearer <token>`
- **Basic Auth**: `Authorization: Basic <base64-encoded-credentials>`
- **API Key**: Via header, query parameter, or cookie

### Response Format
All responses are in JSON format with consistent error handling:

```json
{
  "status": "success|error",
  "data": {...},
  "error": "error message (if status=error)",
  "timestamp": "ISO 8601 timestamp"
}
```

### Error Codes
- `200 OK` - Success
- `400 Bad Request` - Invalid request parameters
- `401 Unauthorized` - Authentication required
- `403 Forbidden` - Insufficient permissions
- `404 Not Found` - Resource not found
- `500 Internal Server Error` - Server error

## Core Concepts

### Adapters
Adapters are the primary integration point for MCP servers. Each adapter represents a configured MCP server instance with:
- Connection type (LocalStdio, RemoteHttp, StreamableHttp, SSE)
- Authentication configuration
- MCP server configuration
- Resource management

### MCP Protocol Support
The API fully implements the Model Context Protocol v1.0:
- **Tools**: Executable functions provided by MCP servers
- **Resources**: Data sources accessible via URI
- **Prompts**: Reusable prompt templates
- **Sampling**: Model interaction capabilities

### Transport Protocols
- **HTTP**: Standard REST endpoints for MCP operations
- **SSE**: Server-sent events for real-time updates
- **WebSocket**: Bidirectional communication for complex interactions
- **Stdio**: Local process communication for development

## Quick Start

### 1. Create an Adapter
```bash
curl -X POST http://localhost:8911/api/v1/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-mcp-server",
    "connectionType": "RemoteHttp",
    "apiBaseUrl": "https://api.example.com/mcp",
    "authentication": {
      "required": true,
      "type": "bearer",
      "bearerToken": {
        "token": "your-token-here"
      }
    }
  }'
```

### 2. List Available Tools
```bash
curl http://localhost:8911/api/v1/adapters/my-mcp-server/tools \
  -H "Authorization: Bearer your-token-here"
```

### 3. Call a Tool
```bash
curl -X POST http://localhost:8911/api/v1/adapters/my-mcp-server/tools/search/call \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-token-here" \
  -d '{"query": "latest news", "limit": 10}'
```

### 4. Access Resources
```bash
curl http://localhost:8911/api/v1/adapters/my-mcp-server/resources \
  -H "Authorization: Bearer your-token-here"
```