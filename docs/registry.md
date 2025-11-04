# MCP Server Registry

The SUSE AI Universal Proxy includes a comprehensive MCP (Model Context Protocol) server registry that allows you to discover, manage, and deploy MCP servers from multiple sources.

## Overview

The registry system supports multiple sources of MCP servers:

- **Official MCP Registry**: 941+ community-contributed servers from the official [Model Context Protocol registry](https://registry.modelcontextprotocol.io)
- **Docker MCP Registry**: 202+ servers from Docker Hub's `mcp` namespace including official and community implementations
- **Custom Sources**: Upload your own MCP server configurations
- **Network Discovery**: Automatically discover MCP servers on your network

**Total Available**: 1,143+ MCP servers across all sources

## Registry Sources

### Official MCP Registry

The official registry contains community-contributed MCP servers published by developers worldwide. These servers are validated and include metadata about their capabilities, installation methods, and requirements.

**Source Identification**: `{"source": "official-mcp"}` in the `_meta` field

### Docker MCP Registry

Docker MCP servers are community-contributed implementations published to Docker Hub under the `mcp` namespace. These include both official Anthropic implementations and third-party servers.

**Source Identification**: `{"source": "docker-mcp"}` in the `_meta` field

**Icon URLs**: Docker servers include Docker Scout security badge URLs in the `icon_url` field, providing visual indicators of container security status.

**Available Docker Servers**: 202+ servers including:
- `mcp/filesystem` - File system operations
- `mcp/everything` - All MCP features demonstration
- `mcp/memory` - In-memory data storage
- `mcp/sequentialthinking` - Sequential reasoning tools
- `mcp/git` - Git repository operations
- `mcp/github` - GitHub API integration
- `mcp/postgres` - PostgreSQL database access
- And 195+ more community servers

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

## Deployment Endpoints

The registry includes deployment capabilities for MCP servers, allowing you to deploy Docker-based MCP servers directly to Kubernetes clusters.

### Get Configuration Template

Retrieve the deployment configuration template for an MCP server, including required environment variables and transport settings.

```http
GET /deployment/config/{serverId}
```

**Response**:
```json
{
  "command": "docker",
  "args": ["run", "--rm", "-i", "mcp/filesystem"],
  "env": {
    "API_KEY": "",
    "SECRET_TOKEN": ""
  },
  "transport": "stdio",
  "image": "mcp/filesystem"
}
```

**Example**:
```bash
curl http://localhost:8911/deployment/config/docker-mcp-filesystem
```

### Deploy MCP Server

Deploy an MCP server to Kubernetes with provided configuration.

```http
POST /deployment/deploy
Content-Type: application/json

{
  "server_id": "docker-mcp-filesystem",
  "env_vars": {
    "API_KEY": "your-api-key-here",
    "SECRET_TOKEN": "your-secret-token"
  },
  "replicas": 1,
  "resources": {
    "cpu": "500m",
    "memory": "512Mi"
  }
}
```

**Response**:
```json
{
  "server_id": "docker-mcp-filesystem",
  "deployment_id": "mcp-docker-mcp-filesystem-123456",
  "status": "deployed"
}
```

**Required Environment Variables**: All environment variables defined in the configuration template must be provided. Missing variables will result in a 400 Bad Request error.

**Transport Types**:
- **stdio**: Deploys as a Deployment with no Service (local communication)
- **http/sse**: Deploys as a Deployment with a ClusterIP Service for network access

**Example Deployment**:
```bash
# Get configuration template
CONFIG=$(curl http://localhost:8911/deployment/config/docker-mcp-brave-search)

# Deploy with environment variables
curl -X POST http://localhost:8911/deployment/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "server_id": "docker-mcp-brave-search",
    "env_vars": {
      "BRAVE_API_KEY": "your-brave-api-key"
    }
  }'
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

### Current Status

- **Official Registry**: 941 servers synced and available
- **Docker Registry**: 202 servers synced and available
- **Total**: 1,143+ MCP servers ready for deployment

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
    "icon_url": "https://api.scout.docker.com/v1/policy/insights/org-image-score/badge/mcp/server-name",
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

### Configuration Template Format

Docker-based MCP servers include a `config_template` field that provides deployment configuration extracted from Docker Hub documentation.

```json
{
  "command": "docker",
  "args": ["run", "--rm", "-i", "mcp/server-name"],
  "env": {
    "API_KEY": "",
    "SECRET_TOKEN": "",
    "DATABASE_URL": ""
  },
  "transport": "stdio",
  "image": "mcp/server-name"
}
```

**Fields**:
- **command**: The command to run (usually "docker")
- **args**: Command arguments including the Docker run command and image
- **env**: Required environment variables (empty values indicate user must provide)
- **transport**: Communication protocol ("stdio", "http", "sse")
- **image**: Docker image name for deployment

**Environment Variable Extraction**: The system automatically extracts required environment variables from Docker Hub README files and configuration examples, identifying API keys, tokens, and other secrets that need to be provided during deployment.

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