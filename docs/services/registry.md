# SUSE AI Uniproxy Registry Service

## Overview

The Registry service manages the catalog of MCP (Model Context Protocol) servers available for deployment. It provides a curated collection of 300+ MCP servers from mcpservers.org and other sources, with metadata, configuration templates, and deployment instructions.

## Architecture

The Registry service runs on port 8913 (HTTP) and 38913 (HTTPS) and provides:

- **Server Catalog**: Curated collection of MCP servers
- **Metadata Management**: Server descriptions, capabilities, and requirements
- **Configuration Templates**: Pre-configured setup for popular MCP servers
- **Sync Operations**: Automatic updates from official registries
- **Search & Discovery**: Server lookup and filtering capabilities

## Core Components

### MCP Server Store
- Persistent storage for MCP server metadata
- Version management and update tracking
- Configuration validation and schema enforcement

### Registry Manager
- Server catalog management and organization
- Category-based classification and tagging
- Dependency resolution and compatibility checking

### Sync Manager
- Automated synchronization with mcpservers.org
- Official registry updates and change detection
- Conflict resolution for duplicate entries

### Search Engine
- Full-text search across server metadata
- Category and tag-based filtering
- Popularity and rating-based sorting

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `REGISTRY_PORT` | `8913` | HTTP server port |
| `REGISTRY_HTTPS_PORT` | `38913` | HTTPS server port |
| `REGISTRY_DATA_DIR` | `./data/registry` | Data storage directory |
| `REGISTRY_SYNC_INTERVAL` | `24h` | Sync interval for official registry |
| `REGISTRY_MAX_SERVERS` | `1000` | Maximum servers to store |

### Command Line Options

```bash
./suse-ai-up-registry [flags]

Flags:
  -port int        Server port (default 8913)
  -data-dir string Data directory (default "./data/registry")
  -sync-interval duration Sync interval (default 24h)
  -max-servers int Maximum servers (default 1000)
```

## API Endpoints

### Server Catalog
- `GET /api/v1/registry` - List all MCP servers
- `GET /api/v1/registry/{id}` - Get server details
- `PUT /api/v1/registry/{id}` - Update server metadata
- `DELETE /api/v1/registry/{id}` - Remove server

### Registry Management
- `GET /api/v1/registry/public` - List public/official servers
- `POST /api/v1/registry/sync/official` - Sync with official registry
- `POST /api/v1/registry/upload` - Upload custom server
- `POST /api/v1/registry/upload/bulk` - Bulk upload servers
- `POST /api/v1/registry/upload/local-mcp` - Upload local MCP server

### Search & Discovery
- `GET /api/v1/registry/browse` - Browse with filtering
- `GET /api/v1/registry/search?q={query}` - Search servers
- `GET /api/v1/registry/categories` - List categories
- `GET /api/v1/registry/tags` - List tags

## Server Metadata Structure

Each MCP server in the registry contains:

```json
{
  "name": "github",
  "image": "mcp/github",
  "type": "server",
  "meta": {
    "category": "productivity",
    "tags": ["git", "github", "collaboration"],
    "sidecarConfig": {
      "commandType": "docker",
      "command": "docker",
      "args": ["run", "-i", "--rm", "mcp/github"],
      "port": 8000,
      "source": "registry"
    }
  },
  "about": {
    "title": "GitHub MCP Server",
    "description": "Access GitHub repositories, issues, and pull requests",
    "icon": "https://github.com/favicon.ico"
  },
  "source": {
    "project": "https://github.com/modelcontextprotocol/server-github",
    "commit": "abc123",
    "branch": "main"
  },
  "config": {
    "description": "GitHub API configuration",
    "secrets": [
      {
        "env": "GITHUB_TOKEN",
        "name": "github.token",
        "example": "ghp_..."
      }
    ],
    "parameters": {
      "properties": {
        "repository": {
          "type": "string",
          "description": "GitHub repository (owner/repo)"
        }
      }
    }
  }
}
```

## Registry Categories

### Development Tools
- **Git**: Version control and repository management
- **GitHub/GitLab**: Issue tracking and code review
- **Docker**: Container management and orchestration
- **Kubernetes**: Cluster management and deployment

### Productivity
- **Notion**: Document collaboration and knowledge management
- **Slack/Discord**: Team communication and automation
- **Google Workspace**: Email, calendar, and document integration
- **Microsoft 365**: Office suite integration

### Data & Analytics
- **PostgreSQL/MySQL**: Database query and management
- **Elasticsearch**: Search and analytics
- **Redis**: Caching and data structures
- **MongoDB**: Document database operations

### AI & ML
- **OpenAI**: GPT model integration
- **Anthropic**: Claude model access
- **HuggingFace**: Model repository and inference
- **LangChain**: LLM application framework

### Cloud Services
- **AWS**: Cloud infrastructure management
- **Azure**: Microsoft cloud services
- **GCP**: Google cloud platform
- **DigitalOcean**: Cloud hosting and management

## Sync Operations

### Official Registry Sync
The registry automatically syncs with mcpservers.org:

```bash
# Manual sync
curl -X POST http://localhost:8913/api/v1/registry/sync/official

# Check sync status
curl http://localhost:8913/api/v1/registry/sync/status
```

### Sync Process
1. **Discovery**: Fetch latest server catalog from official registry
2. **Validation**: Verify server metadata and configurations
3. **Merge**: Update existing servers and add new ones
4. **Cleanup**: Remove deprecated or invalid entries
5. **Notification**: Log sync results and statistics

## Custom Server Upload

### Single Server Upload
```bash
curl -X POST http://localhost:8913/api/v1/registry/upload \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-custom-server",
    "image": "myorg/my-server:latest",
    "type": "server",
    "meta": {
      "category": "custom",
      "tags": ["internal", "custom"],
      "sidecarConfig": {
        "commandType": "docker",
        "command": "docker",
        "args": ["run", "-p", "8000:8000", "myorg/my-server:latest"],
        "port": 8000
      }
    },
    "config": {
      "secrets": [
        {
          "env": "API_KEY",
          "name": "my-server.api_key"
        }
      ]
    }
  }'
```

### Bulk Upload
```bash
curl -X POST http://localhost:8913/api/v1/registry/upload/bulk \
  -H "Content-Type: application/json" \
  -d '[
    {"name": "server1", ...},
    {"name": "server2", ...}
  ]'
```

### Local MCP Server
```bash
curl -X POST http://localhost:8913/api/v1/registry/upload/local-mcp \
  -F "file=@package.json" \
  -F "config={\"name\": \"local-server\", \"version\": \"1.0.0\"}"
```

## Search & Filtering

### Basic Search
```bash
# Search by name or description
curl "http://localhost:8913/api/v1/registry/search?q=github"

# Search by category
curl "http://localhost:8913/api/v1/registry/browse?category=productivity"

# Search by tags
curl "http://localhost:8913/api/v1/registry/browse?tags=git,docker"
```

### Advanced Filtering
```bash
# Multiple filters
curl "http://localhost:8913/api/v1/registry/browse?category=development&tags=docker,kubernetes&limit=10&offset=0"

# Sort options
curl "http://localhost:8913/api/v1/registry/browse?sort=popularity&order=desc"
```

## Server Validation

### Configuration Validation
- Schema validation for server metadata
- Configuration parameter validation
- Dependency checking and compatibility
- Security scanning for malicious configurations

### Health Checks
- Server image availability verification
- Configuration template validation
- Network connectivity testing
- Performance benchmarking

## Caching & Performance

### Metadata Caching
- Server metadata cached for fast access
- Automatic cache invalidation on updates
- Distributed cache support for scalability

### Search Optimization
- Full-text search indexing
- Query result caching
- Pagination and result limiting
- Response compression

## Security Features

### Access Control
- Registry access authentication
- Server upload authorization
- Administrative operation restrictions
- Audit logging for all operations

### Content Security
- Server configuration sanitization
- Malicious code detection
- Safe image source validation
- Configuration template security scanning

## Monitoring & Observability

### Metrics
- Server catalog size and growth
- Sync operation statistics
- Search query performance
- API usage patterns

### Health Monitoring
- Registry service availability
- Data consistency checks
- Sync operation status
- Storage capacity monitoring

### Logging
- Structured logging for all operations
- Audit trails for configuration changes
- Error tracking and alerting
- Performance monitoring

## Integration Examples

### Registry Browser
```javascript
class RegistryBrowser {
  constructor(baseUrl = 'http://localhost:8913') {
    this.baseUrl = baseUrl;
  }

  async searchServers(query, category = null) {
    const params = new URLSearchParams({ q: query });
    if (category) params.set('category', category);

    const response = await fetch(`${this.baseUrl}/api/v1/registry/search?${params}`);
    return response.json();
  }

  async getServerDetails(serverId) {
    const response = await fetch(`${this.baseUrl}/api/v1/registry/${serverId}`);
    return response.json();
  }

  async getCategories() {
    const response = await fetch(`${this.baseUrl}/api/v1/registry/categories`);
    return response.json();
  }
}
```

### Server Deployer
```python
import requests
import json

class ServerDeployer:
    def __init__(self, registry_url='http://localhost:8913', uniproxy_url='http://localhost:8911'):
        self.registry_url = registry_url
        self.uniproxy_url = uniproxy_url

    def find_server(self, name):
        """Find server in registry"""
        response = requests.get(f"{self.registry_url}/api/v1/registry/search?q={name}")
        servers = response.json()
        return servers[0] if servers else None

    def deploy_server(self, server_name, config=None):
        """Deploy server as adapter"""
        server = self.find_server(server_name)
        if not server:
            raise ValueError(f"Server {server_name} not found")

        # Merge configurations
        adapter_config = {
            "name": f"{server_name}-adapter",
            "connectionType": "LocalStdio",
            "mcpClientConfig": server.get("mcpClientConfig", {}),
            "authentication": config.get("authentication") if config else None
        }

        # Create adapter
        response = requests.post(
            f"{self.uniproxy_url}/api/v1/adapters",
            json=adapter_config
        )
        return response.json()
```

## Best Practices

### Registry Management
- Regular sync with official registry
- Monitor for deprecated servers
- Validate custom server uploads
- Maintain backup of registry data

### Performance Optimization
- Implement caching for frequently accessed servers
- Use pagination for large result sets
- Optimize search queries
- Monitor storage usage

### Security
- Validate all server configurations
- Implement access controls
- Regular security audits
- Monitor for suspicious activity

### Maintenance
- Regular data cleanup
- Monitor sync operation health
- Update server metadata
- Archive unused servers

## Troubleshooting

### Sync Issues
```bash
# Check sync status
curl http://localhost:8913/api/v1/registry/sync/status

# Force manual sync
curl -X POST http://localhost:8913/api/v1/registry/sync/official

# Check logs
tail -f /var/log/suse-ai-up/registry.log
```

### Search Problems
```bash
# Rebuild search index
curl -X POST http://localhost:8913/api/v1/registry/search/rebuild

# Check search health
curl http://localhost:8913/api/v1/registry/search/health
```

### Storage Issues
```bash
# Check storage usage
curl http://localhost:8913/api/v1/registry/storage/stats

# Cleanup old data
curl -X POST http://localhost:8913/api/v1/registry/storage/cleanup
```

## API Versioning

The Registry service uses API versioning with the `/api/v1/` prefix. Future versions will maintain backward compatibility and provide migration guides for breaking changes.