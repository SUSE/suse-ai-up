# MCP Server Registry

The SUSE AI Universal Proxy includes a comprehensive MCP (Model Context Protocol) server registry that allows you to discover, manage, and deploy MCP servers from multiple sources.

## Overview

The registry system supports multiple sources of MCP servers:

- **Official MCP Registry**: Community-contributed servers from the official [Model Context Protocol registry](https://registry.modelcontextprotocol.io)
- **Docker MCP Registry**: Official MCP server implementations from the [modelcontextprotocol/servers](https://github.com/modelcontextprotocol/servers) repository
- **Custom Sources**: Upload your own MCP server configurations
- **Network Discovery**: Automatically discover MCP servers on your network

## Registry Sources

### Official MCP Registry

The official registry contains community-contributed MCP servers published by developers worldwide. These servers are validated and include metadata about their capabilities, installation methods, and requirements.

**Source Identification**: `{"source": "official-mcp"}` in the `_meta` field

### Docker MCP Registry

Docker MCP servers are the official reference implementations maintained by Anthropic. These are production-ready servers that implement the MCP protocol for various use cases.

**Source Identification**: `{"source": "docker-mcp"}` in the `_meta` field

**Available Docker Servers**:
- `@modelcontextprotocol/server-filesystem` - File system operations
- `@modelcontextprotocol/server-everything` - All MCP features demonstration
- `@modelcontextprotocol/server-memory` - In-memory data storage
- `@modelcontextprotocol/server-sequential-thinking` - Sequential reasoning tools

## API Endpoints

### Browse Registry

Search and filter MCP servers from all configured sources.

```http
GET /registry/browse?q=search_term&registryType=oci&transport=stdio
```

**Query Parameters**:
- `q`: Search query (searches names and descriptions)
- `transport`: Filter by transport type (`stdio`, `sse`, `websocket`, `http`)
- `registryType`: Filter by registry type (`oci`, `npm`, `mcpb`)
- `validationStatus`: Filter by validation status (`new`, `approved`, `certified`)

**Example**:
```bash
# Search for filesystem servers
curl "http://localhost:8911/registry/browse?q=filesystem"

# Filter by Docker registry type
curl "http://localhost:8911/registry/browse?registryType=oci"

# Search Docker MCP servers
curl "http://localhost:8911/registry/browse?q=docker-mcp"
```

### Get Server Details

Retrieve detailed information about a specific MCP server.

```http
GET /registry/{id}
```

**Example**:
```bash
curl http://localhost:8911/registry/docker-1
```

### Upload Single Server

Upload a single MCP server configuration.

```http
POST /registry/upload
Content-Type: application/json

{
  "id": "my-custom-server",
  "name": "My Custom MCP Server",
  "description": "A custom MCP server implementation",
  "version": "1.0.0",
  "packages": [
    {
      "registryType": "npm",
      "identifier": "my-mcp-server",
      "transport": {
        "type": "stdio"
      }
    }
  ],
  "repository": {
    "url": "https://github.com/user/my-mcp-server",
    "source": "github"
  },
  "validation_status": "new",
  "_meta": {
    "source": "custom"
  }
}
```

### Bulk Upload

Upload multiple MCP server configurations at once.

```http
POST /registry/upload/bulk
Content-Type: application/json

[
  {
    "id": "server-1",
    "name": "Server One",
    "description": "First server",
    "packages": [...],
    "_meta": {"source": "custom"}
  },
  {
    "id": "server-2",
    "name": "Server Two",
    "description": "Second server",
    "packages": [...],
    "_meta": {"source": "custom"}
  }
]
```

### Sync Official Registry

Manually trigger synchronization with the official MCP registry.

```http
POST /registry/sync/official
```

### Update Server

Update an existing MCP server configuration.

```http
PUT /registry/{id}
Content-Type: application/json

{
  "name": "Updated Server Name",
  "description": "Updated description",
  "validation_status": "approved"
}
```

### Delete Server

Remove an MCP server from the registry.

```http
DELETE /registry/{id}
```

## Registry Management Scripts

The repository includes scripts to help manage the registry:

### Sync Official Registry

```bash
# Run the official registry sync script
go run search_mcp_registry.go

# Publish the found servers
go run scripts/publish_servers.go
```

### Sync Docker Registry

```bash
# Run the Docker registry sync script
go run scripts/search_docker_registry.go

# Publish the Docker servers
go run scripts/publish_servers.go docker
```

## Server Configuration Format

### MCPServer Schema

```json
{
  "id": "unique-server-id",
  "name": "Human-readable server name",
  "description": "Detailed server description",
  "version": "1.0.0",
  "repository": {
    "url": "https://github.com/user/repo",
    "source": "github"
  },
  "packages": [
    {
      "registryType": "npm",
      "identifier": "package-name",
      "transport": {
        "type": "stdio"
      },
      "environmentVariables": [
        {
          "name": "API_KEY",
          "description": "API key for authentication",
          "isSecret": true
        }
      ]
    }
  ],
  "tools": [
    {
      "name": "tool_name",
      "description": "Tool description",
      "input_schema": {
        "type": "object",
        "properties": {
          "param1": {"type": "string"}
        }
      }
    }
  ],
  "validation_status": "new",
  "discovered_at": "2024-01-01T00:00:00Z",
  "_meta": {
    "source": "official-mcp",
    "custom_field": "value"
  }
}
```

### Package Types

- **npm**: Node.js packages from npm registry
- **oci**: Container images (Docker)
- **mcpb**: MCP binary releases
- **pypi**: Python packages

### Transport Types

- **stdio**: Standard input/output communication
- **sse**: Server-sent events
- **websocket**: WebSocket communication
- **http**: HTTP-based communication

## Examples

### 1. Browse All Servers

```bash
curl http://localhost:8911/registry/browse | jq '.[] | {name, description, _meta}'
```

### 2. Find Docker Servers

```bash
curl "http://localhost:8911/registry/browse" | jq '.[] | select(._meta.source == "docker-mcp") | {name, description}'
```

### 3. Search for File Operations

```bash
curl "http://localhost:8911/registry/browse?q=file" | jq '.[] | {name, packages}'
```

### 4. Upload Custom Server

```bash
curl -X POST http://localhost:8911/registry/upload \
  -H "Content-Type: application/json" \
  -d '{
    "id": "my-git-server",
    "name": "My Git MCP Server",
    "description": "Custom Git operations server",
    "packages": [{
      "registryType": "npm",
      "identifier": "my-git-mcp",
      "transport": {"type": "stdio"}
    }],
    "_meta": {"source": "custom"}
  }'
```

### 5. Register Docker Server as Adapter

First, find the server ID:

```bash
curl "http://localhost:8911/registry/browse?q=filesystem" | jq '.[0].id'
```

Then register it as an adapter:

```bash
curl -X POST http://localhost:8911/register \
  -H "Content-Type: application/json" \
  -d '{"discoveredServerId": "docker-2"}'
```

## Registry Architecture

### Storage

The registry uses an in-memory store for MCP server configurations. This means:

- Servers persist across requests but not service restarts
- Fast access and search capabilities
- Easy to extend with persistent storage backends

### Synchronization

- **Official Registry**: Periodic sync with configurable intervals
- **Docker Registry**: Manual sync via scripts
- **Custom Sources**: HTTP/HTTPS URLs or local files

### Validation

Servers can have different validation statuses:

- **new**: Recently added, not yet validated
- **approved**: Reviewed and approved for use
- **certified**: Thoroughly tested and certified

## Troubleshooting

### Common Issues

1. **Server not found after upload**
   - Check that the server ID is unique
   - Verify the JSON format is correct

2. **Sync failures**
   - Check network connectivity
   - Verify API endpoints are accessible

3. **Search not returning expected results**
   - Use exact terms or check spelling
   - Try broader search terms

### Debug Commands

```bash
# Check total server count
curl -s http://localhost:8911/registry/browse | jq 'length'

# Check server sources
curl -s http://localhost:8911/registry/browse | jq 'group_by(._meta.source) | map({(.[0]._meta.source): length}) | add'

# Validate server configuration
curl http://localhost:8911/registry/{id} | jq .
```

## Contributing

To add new registry sources or improve the registry functionality:

1. Implement the `RegistryManagerInterface` for new sources
2. Add appropriate sync logic
3. Update the API documentation
4. Test with various server configurations

The registry system is designed to be extensible and can easily accommodate new MCP server sources and validation mechanisms.</content>
</xai:function_call="write">
<parameter name="filePath">docs/registry.md