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
curl "http://localhost:8911/api/v1/registry/browse?q=filesystem"

# Filter by Docker registry type
curl "http://localhost:8911/api/v1/registry/browse?registryType=oci"

# Search Docker MCP servers
curl "http://localhost:8911/api/v1/registry/browse?q=docker-mcp"
```

### Get Server Details

Retrieve detailed information about a specific MCP server.

```http
GET /registry/{id}
```

**Example**:
```bash
curl http://localhost:8911/api/v1/registry/docker-1
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



**Response**:
```json
{
  "message": "VirtualMCP adapter created and deployed successfully",
  "adapter": {
    "id": "virtualmcp-filesystem-123",
    "name": "virtualmcp-filesystem",
    "connectionType": "StreamableHttp",
    "description": "VirtualMCP adapter for filesystem"
  },
  "mcp_endpoint": "http://localhost:8911/api/v1/adapters/virtualmcp-filesystem-123/mcp",
  "token_info": {
    "token": "eyJhbGciOiJIUzI1NiIs...",
    "tokenType": "Bearer",
    "expiresAt": "2024-12-04T10:00:00Z"
  },
  "note": "VirtualMCP server is now running and ready to use"
}
```

### Spawning Behavior

- **Automatic Deployment**: Registry entries are automatically spawned as running processes when adapters are created
- **Type Detection**: Automatically determines server type (VirtualMCP, LocalStdio, RemoteHTTP) and applies appropriate spawning logic
- **Resource Limits**: Configurable CPU and memory limits prevent resource exhaustion
- **Retry Logic**: Failed spawns are retried with exponential backoff
- **Error Handling**: Spawning failures are logged but don't create adapters (fail-fast approach)
- **Tool Discovery**: Tools are discovered at runtime, not pre-defined in registry

### Server Types

#### VirtualMCP Servers
- **Deployment**: Spawned as local HTTP processes with authentication
- **Transport**: HTTP with Bearer token authentication
- **Use Case**: Custom MCP servers with tool definitions
- **Example**: Official MCP servers like filesystem, git, memory

#### LocalStdio Servers
- **Deployment**: Spawned on-demand as subprocesses
- **Transport**: Standard input/output communication
- **Use Case**: CLI tools and utilities
- **Example**: npm-based MCP servers

#### RemoteHTTP Servers
- **Deployment**: Proxy to remote HTTP endpoints
- **Transport**: HTTP with optional authentication
- **Use Case**: Externally hosted MCP servers

### Configuration

Spawning behavior is controlled by environment variables:

```bash
# Retry configuration
SPAWNING_RETRY_ATTEMPTS=3
SPAWNING_RETRY_BACKOFF_MS=2000

# Resource limits (defaults)
SPAWNING_DEFAULT_CPU=500m
SPAWNING_DEFAULT_MEMORY=256Mi
SPAWNING_MAX_CPU=1000m
SPAWNING_MAX_MEMORY=1Gi

# Logging
SPAWNING_LOG_LEVEL=debug
SPAWNING_INCLUDE_CONTEXT=true
```

### Pre-loaded Servers

The system includes pre-loaded official MCP servers that are immediately available:

- **filesystem**: Secure file operations
- **git**: Git repository tools
- **memory**: Knowledge graph memory
- **sequential-thinking**: Problem-solving tools
- **time**: Timezone conversion
- **everything**: Reference implementation
- **fetch**: Web content fetching

### Spawning Examples

#### Spawn Official Filesystem Server

```bash
# Create adapter (automatically spawns server)
curl -X POST http://localhost:8911/api/v1/registry/filesystem/create-adapter \
  -H "Content-Type: application/json" \
  -d '{
    "environmentVariables": {
      "ALLOWED_DIRS": "/tmp,/home/user"
    }
  }'
```

#### Spawn with Custom Environment

```bash
curl -X POST http://localhost:8911/api/v1/registry/memory/create-adapter \
  -H "Content-Type: application/json" \
  -d '{
    "replicaCount": 1,
    "environmentVariables": {
      "MAX_MEMORY_ITEMS": "1000",
      "PERSISTENCE_FILE": "/data/memory.json"
    }
  }'
```

#### Check Spawning Status

```bash
# List all registry servers
curl http://localhost:8911/api/v1/registry/browse

# Get server details including config template
curl http://localhost:8911/api/v1/registry/filesystem

# Check running processes (if exposed via API)
curl http://localhost:8911/api/v1/deployment/processes
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
curl http://localhost:8911/api/v1/deployment/config/docker-mcp-filesystem
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
CONFIG=$(curl http://localhost:8911/api/v1/deployment/config/docker-mcp-brave-search)

# Deploy with environment variables
curl -X POST http://localhost:8911/api/v1/deployment/deploy \
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
curl http://localhost:8911/api/v1/registry/browse | jq '.[] | {name, description, _meta}'
```

### 2. Find Docker Servers

```bash
curl "http://localhost:8911/api/v1/registry/browse" | jq '.[] | select(._meta.source == "docker-mcp") | {name, description}'
```

### 3. Search for File Operations

```bash
curl "http://localhost:8911/api/v1/registry/browse?q=file" | jq '.[] | {name, packages}'
```

### 4. Upload Custom Server

```bash
curl -X POST http://localhost:8911/api/v1/registry/upload \
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
curl "http://localhost:8911/api/v1/registry/browse?q=filesystem" | jq '.[0].id'
```

Then register it as an adapter:

```bash
curl -X POST http://localhost:8911/api/v1/discovery/register \
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
curl -s http://localhost:8911/api/v1/registry/browse | jq 'length'

# Check server sources
curl -s http://localhost:8911/api/v1/registry/browse | jq 'group_by(._meta.source) | map({(.[0]._meta.source): length}) | add'

# Validate server configuration
curl http://localhost:8911/api/v1/registry/{id} | jq .
```

## UI Development Guide

This section provides information needed to build user interfaces around the MCP server registry and spawning functionality.

### Registry Browser UI

#### Server List Display

**API Endpoint**: `GET /registry/browse`

**Response Structure**:
```json
[
  {
    "id": "filesystem",
    "name": "filesystem",
    "description": "Secure file operations with configurable access controls",
    "version": "latest",
    "packages": [
      {
        "registryType": "npm",
        "identifier": "@modelcontextprotocol/server-filesystem",
        "transport": {"type": "stdio"}
      }
    ],
    "validation_status": "approved",
    "discovered_at": "2024-12-03T00:00:00Z",
    "_meta": {
      "source": "official"
    },
    "config_template": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem"],
      "transport": "stdio",
      "resource_limits": {
        "cpu": "500m",
        "memory": "256Mi"
      }
    }
  }
]
```

**UI Elements to Display**:
- Server name and description
- Source badge (official, docker-mcp, custom)
- Transport type icon (stdio, http, sse)
- Validation status indicator


#### Server Detail View

**API Endpoint**: `GET /registry/{id}`

**UI Components**:
- Server metadata (name, description, version)
- Package information with installation commands
- Configuration template with environment variables
- Spawn form with environment variable inputs
- Resource limit display
- Tool discovery status



### Error Handling UI



### Configuration UI

#### Spawning Settings

**Environment Variables**:
- `SPAWNING_RETRY_ATTEMPTS`: Number input (1-10)
- `SPAWNING_RETRY_BACKOFF_MS`: Number input (500-10000)
- `SPAWNING_DEFAULT_CPU`: Text input (e.g., "500m")
- `SPAWNING_DEFAULT_MEMORY`: Text input (e.g., "256Mi")
- `SPAWNING_LOG_LEVEL`: Select (debug, info, warn, error)

**Validation Rules**:
- CPU: Valid Kubernetes CPU units (m, empty for cores)
- Memory: Valid Kubernetes memory units (Mi, Gi, etc.)
- Retry attempts: Positive integer
- Backoff: Positive integer in milliseconds

### Real-time Updates

#### WebSocket/SSE Integration

For real-time spawning status updates:

```javascript
// Connect to spawning status endpoint
const eventSource = new EventSource('/api/v1/spawning/status');

// Listen for spawning events
eventSource.addEventListener('spawning', (event) => {
  const data = JSON.parse(event.data);
  // Update UI with spawning progress
  updateSpawningStatus(data.serverId, data.status, data.progress);
});
```

#### Server Health Monitoring

**Process Status API** (if implemented):
```json
{
  "serverId": "filesystem",
  "status": "running",
  "port": 8080,
  "cpuUsage": "45m",
  "memoryUsage": "120Mi",
  "uptime": "300s"
}
```

### UI Component Specifications

#### Server Card Component

```jsx
<ServerCard
  server={server}
/>
```

**Props**:
- `server`: Server object from API

#### Status Notification System

```jsx
<NotificationSystem>
  <Notification
    type="success"
    message="Server spawned successfully"
    details={{
      endpoint: mcpEndpoint,
      token: authToken
    }}
  />
</NotificationSystem>
```

### Accessibility Considerations

- Keyboard navigation for all spawning controls
- Screen reader support for server descriptions
- High contrast mode support for status indicators
- Error messages with actionable guidance

### Performance Optimization

- Virtual scrolling for large server lists
- Lazy loading of server details
- Debounced search input
- Caching of frequently accessed server data

## Contributing

To add new registry sources or improve the registry functionality:

1. Implement the `RegistryManagerInterface` for new sources
2. Add appropriate sync logic
3. Update the API documentation
4. Test with various server configurations

The registry system is designed to be extensible and can easily accommodate new MCP server sources and validation mechanisms.</content>
</xai:function_call="write">
<parameter name="filePath">docs/registry.md