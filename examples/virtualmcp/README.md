# VirtualMCP Service

The VirtualMCP service provides MCP (Model Context Protocol) implementation discovery and management. It exposes available MCP server implementations that can be automatically registered in the SUSE AI Universal Proxy registry.

## Features

- **MCP Implementation Registry**: Maintains a catalog of available MCP server implementations
- **Auto-Registration**: Automatically registers with the proxy when started
- **REST API**: Provides endpoints for MCP implementation discovery and management
- **Default Implementations**: Includes pre-configured implementations for popular AI services

## Default MCP Implementations

The service comes with several pre-configured MCP implementations:

- **Anthropic Claude 3**: Claude 3 model via Anthropic API
- **OpenAI GPT-4**: GPT-4 model via OpenAI API
- **Local Filesystem**: Access to local filesystem operations

## API Endpoints

### Health Check
```
GET /health
```
Returns service health status and implementation count.

### MCP Implementation Discovery
```
GET /api/v1/mcps
```
Returns all available MCP implementations.

**Response:**
```json
{
  "implementations": [...],
  "count": 3,
  "service": "virtualmcp-abc123"
}
```

### Get Specific Implementation
```
GET /api/v1/mcps/{id}
```
Returns details for a specific MCP implementation.

### Add Implementation
```
POST /api/v1/mcps
```
Add a new MCP implementation.

**Request Body:**
```json
{
  "name": "My Custom MCP",
  "description": "Custom MCP implementation",
  "transport": "stdio",
  "capabilities": ["chat", "tools"],
  "config_template": {
    "command": "python",
    "args": ["my_server.py"],
    "env": {"API_KEY": ""}
  }
}
```

### Update Implementation
```
PUT /api/v1/mcps/{id}
```
Update an existing MCP implementation.

### Delete Implementation
```
DELETE /api/v1/mcps/{id}
```
Remove an MCP implementation.

## Configuration

Environment variables:

- `VIRTUALMCP_PORT`: Service port (default: 8913)
- `PROXY_URL`: Proxy service URL (default: http://localhost:8911)
- `SERVICE_ID`: Unique service identifier (auto-generated if not provided)

## Running the Service

1. Install dependencies:
```bash
pip install -r requirements.txt
```

2. Start the service:
```bash
python src/main.py
```

The service will automatically register with the proxy and expose its MCP implementations.

## Integration with Proxy

When the VirtualMCP service starts, it:

1. Registers itself with the proxy as a "virtualmcp" service type
2. Declares capabilities for MCP implementation discovery
3. The proxy automatically discovers available MCP implementations
4. Implementations are registered in the local registry
5. Users can create adapters from the registry entries

This enables seamless discovery and deployment of MCP server implementations through the existing registry and adapter system.