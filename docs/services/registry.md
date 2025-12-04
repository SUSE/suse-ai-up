# MCP Registry Service

The MCP Registry Service is the central catalog for MCP (Model Context Protocol) servers in the SUSE AI Universal Proxy system. It maintains a comprehensive database of available MCP servers, their capabilities, and metadata, enabling automatic discovery and integration.

## Overview

The registry service acts as a centralized repository that:

- **Server Catalog**: Maintains a searchable catalog of MCP servers
- **Multi-Source Sync**: Synchronizes from official and Docker registries
- **Metadata Management**: Stores detailed server information and capabilities
- **Validation Framework**: Tracks server validation status and health
- **API Integration**: Provides RESTful APIs for server management

## Architecture

### Service Position
```
┌─────────────┐
│   CLIENT    │
│  (Browser,  │
│   CLI, IDE) │
└──────┬──────┘
       │
       ▼
┌─────────────┐    ┌─────────────┐
│   PROXY     │◄──►│  REGISTRY   │
│  (Primary)  │    │ (Sidecar)   │
│             │    │             │
│ Port: 8080  │    │ Port: 8913  │
│ HTTPS:38080 │    │ HTTPS:38913 │
└──────┬──────┘    └─────────────┘
       │
       ▼
┌─────────────┐
│ MCP SERVER  │
│  (External) │
└─────────────┘
```

### Key Components

- **Server Store**: In-memory database for MCP server metadata
- **Sync Manager**: Handles synchronization with external registries
- **Validation Engine**: Validates server configurations and capabilities
- **Search Engine**: Provides advanced filtering and search capabilities
- **API Layer**: RESTful endpoints for server management

## Configuration

### Environment Variables

```bash
# Basic Configuration
REGISTRY_PORT=8913              # HTTP port (default: 8913)
TLS_PORT=38913                  # HTTPS port (default: 38913)

# TLS Configuration
AUTO_TLS=true                   # Auto-generate self-signed certificates
TLS_CERT_FILE=/path/to/cert.pem
TLS_KEY_FILE=/path/to/key.pem

# Registry Sources
ENABLE_OFFICIAL=true            # Enable official MCP registry sync
ENABLE_DOCKER=true              # Enable Docker registry sync
SYNC_INTERVAL=24h               # Sync interval (default: 24h)

# Performance Tuning
MAX_SERVERS=10000               # Maximum servers in registry
CACHE_TTL=3600                  # Cache TTL in seconds
```

### Docker Configuration

```yaml
services:
  registry:
    image: suse/suse-ai-up:latest
    ports:
      - "8913:8913"      # HTTP
      - "38913:38913"    # HTTPS
    environment:
      - AUTO_TLS=true
      - ENABLE_OFFICIAL=true
      - ENABLE_DOCKER=true
    command: ["./suse-ai-up", "registry"]
```

### Kubernetes Configuration

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: suse-ai-up-registry
spec:
  template:
    spec:
      containers:
      - name: registry
        image: suse/suse-ai-up:latest
        ports:
        - containerPort: 8913
          name: http
        - containerPort: 38913
          name: https
        env:
        - name: AUTO_TLS
          value: "true"
        - name: ENABLE_OFFICIAL
          value: "true"
        command: ["./suse-ai-up", "registry"]
```

## API Endpoints

### Server Browsing

#### List All Servers
```http
GET /api/v1/registry/browse
```

Query Parameters:
- `q`: Search query (name, description)
- `transport`: Filter by transport type (http, websocket, sse)
- `registryType`: Filter by registry type (official, docker, custom)
- `validationStatus`: Filter by validation status (validated, pending, failed)

Response:
```json
[
  {
    "id": "server-123",
    "name": "Weather API Server",
    "description": "Provides weather data and forecasts",
    "version": "1.0.0",
    "validationStatus": "validated",
    "discoveredAt": "2025-12-04T10:00:00Z",
    "packages": [
      {
        "name": "weather-tools",
        "version": "1.0.0",
        "transport": {
          "type": "http",
          "url": "http://weather-server:8080"
        },
        "registryType": "official"
      }
    ]
  }
]
```

#### Get Server by ID
```http
GET /api/v1/registry/{server_id}
```

#### Update Server
```http
PUT /api/v1/registry/{server_id}
Content-Type: application/json

{
  "name": "Updated Server Name",
  "description": "Updated description",
  "validationStatus": "validated"
}
```

#### Delete Server
```http
DELETE /api/v1/registry/{server_id}
```

### Server Upload

#### Upload Single Server
```http
POST /api/v1/registry/upload
Content-Type: application/json

{
  "name": "Custom MCP Server",
  "description": "My custom server",
  "version": "1.0.0",
  "packages": [
    {
      "name": "custom-tools",
      "version": "1.0.0",
      "transport": {
        "type": "http",
        "url": "http://my-server:8080"
      }
    }
  ]
}
```

#### Bulk Upload Servers
```http
POST /api/v1/registry/upload/bulk
Content-Type: application/json

[
  {
    "name": "Server 1",
    "packages": [...]
  },
  {
    "name": "Server 2",
    "packages": [...]
  }
]
```

### Registry Synchronization

#### Sync Official Registry
```http
POST /api/v1/registry/sync/official
```

Response:
```json
{
  "status": "sync_started",
  "source": "official"
}
```

#### Sync Docker Registry
```http
POST /api/v1/registry/sync/docker
```

### Administrative Endpoints

#### Health Check
```http
GET /health
```

Response:
```json
{
  "status": "healthy",
  "service": "registry",
  "timestamp": "2025-12-04T12:00:00Z",
  "serverCount": 150
}
```

## Registry Sources

### Official MCP Registry

The official registry contains validated, production-ready MCP servers:

- **Source**: Official MCP project registry
- **Sync Frequency**: Daily (configurable)
- **Validation**: Pre-validated servers
- **Content**: Core MCP servers and tools

### Docker Registry

Docker-based MCP servers discovered through container registries:

- **Source**: Docker Hub and private registries
- **Discovery**: Image labels and metadata
- **Validation**: Runtime validation required
- **Content**: Community and custom servers

### Custom Servers

User-uploaded and locally discovered servers:

- **Source**: Manual upload or discovery service
- **Validation**: User-defined validation status
- **Content**: Private and development servers

## Server Metadata

### Server Object Structure

```json
{
  "id": "unique-server-id",
  "name": "Server Display Name",
  "description": "Detailed server description",
  "version": "1.2.3",
  "validationStatus": "validated|pending|failed|new",
  "discoveredAt": "2025-12-04T10:00:00Z",
  "lastValidated": "2025-12-04T11:00:00Z",
  "packages": [
    {
      "name": "package-name",
      "version": "1.0.0",
      "description": "Package description",
      "transport": {
        "type": "http|websocket|sse",
        "url": "endpoint-url",
        "headers": {
          "Authorization": "Bearer token"
        }
      },
      "registryType": "official|docker|custom",
      "capabilities": {
        "tools": ["tool1", "tool2"],
        "resources": ["resource1"],
        "prompts": ["prompt1"]
      }
    }
  ],
  "meta": {
    "source": "official",
    "tags": ["weather", "api"],
    "author": "SUSE",
    "license": "MIT"
  }
}
```

### Validation Status

- **validated**: Server has been tested and confirmed working
- **pending**: Server awaiting validation
- **failed**: Server validation failed
- **new**: Newly discovered, not yet validated

## Search and Filtering

### Search Syntax

The registry supports advanced search and filtering:

```bash
# Search by name or description
GET /api/v1/registry/browse?q=weather

# Filter by transport type
GET /api/v1/registry/browse?transport=http

# Filter by registry type
GET /api/v1/registry/browse?registryType=official

# Combine filters
GET /api/v1/registry/browse?q=api&transport=http&validationStatus=validated
```

### Advanced Filtering

- **Text Search**: Full-text search across name and description
- **Transport Types**: http, websocket, sse
- **Registry Types**: official, docker, custom
- **Validation Status**: validated, pending, failed, new
- **Date Ranges**: discoveredAt, lastValidated

## Synchronization Process

### Sync Workflow

1. **Trigger Sync**: Manual or scheduled sync initiation
2. **Fetch Sources**: Retrieve server lists from registries
3. **Validate Entries**: Check server accessibility and capabilities
4. **Update Database**: Add new servers, update existing ones
5. **Cleanup**: Remove stale or invalid entries

### Sync Configuration

```yaml
sync:
  official:
    enabled: true
    interval: 24h
    url: "https://registry.mcp-project.org/api/v1/servers"
  docker:
    enabled: true
    interval: 6h
    registries:
      - "docker.io"
      - "registry.example.com"
```

## Monitoring & Observability

### Metrics

The registry exposes Prometheus-compatible metrics:

```
# Server counts
mcp_registry_servers_total{status="validated"} 150
mcp_registry_servers_total{status="pending"} 25

# Sync operations
mcp_registry_sync_duration_seconds{source="official"} 2.34
mcp_registry_sync_last_success_timestamp{source="docker"} 1733313600

# API requests
mcp_registry_api_requests_total{method="GET", endpoint="/browse"} 1250
```

### Health Checks

- **Readiness Probe**: Validates database connectivity
- **Liveness Probe**: Checks service responsiveness
- **Dependency Checks**: Validates sync source availability

### Logging

Structured logging with configurable levels:

```json
{
  "timestamp": "2025-12-04T12:00:00Z",
  "level": "info",
  "service": "registry",
  "event": "server_sync_completed",
  "source": "official",
  "server_count": 150,
  "duration_ms": 2340
}
```

## Performance Tuning

### Caching

```bash
export CACHE_ENABLED=true
export CACHE_TTL=3600
export CACHE_SIZE=10000
```

### Database Optimization

```bash
export DB_MAX_CONNECTIONS=100
export DB_CONNECTION_TIMEOUT=30s
export DB_QUERY_TIMEOUT=10s
```

### Sync Optimization

```bash
export SYNC_BATCH_SIZE=50
export SYNC_TIMEOUT=300s
export SYNC_RETRY_ATTEMPTS=3
```

## Troubleshooting

### Common Issues

**Sync Failures**
```bash
# Check sync status
curl http://localhost:8913/health

# Manual sync trigger
curl -X POST http://localhost:8913/api/v1/registry/sync/official

# Check logs
kubectl logs -f deployment/suse-ai-up-registry
```

**Search Not Working**
```bash
# Test search endpoint
curl "http://localhost:8913/api/v1/registry/browse?q=test"

# Check server count
curl http://localhost:8913/health
```

**High Memory Usage**
```bash
# Check cache settings
kubectl exec -it deployment/suse-ai-up-registry -- env | grep CACHE

# Monitor memory usage
kubectl top pods
```

### Debug Mode

Enable detailed logging for troubleshooting:

```bash
export LOG_LEVEL=debug
export LOG_FORMAT=json
./suse-ai-up registry
```

### Data Recovery

```bash
# Export registry data
curl http://localhost:8913/api/v1/registry/browse > registry_backup.json

# Import registry data
curl -X POST http://localhost:8913/api/v1/registry/upload/bulk \
  -H "Content-Type: application/json" \
  -d @registry_backup.json
```

## Security Considerations

### Access Control

- **API Authentication**: Bearer token or API key authentication
- **Rate Limiting**: Prevent abuse of search and upload endpoints
- **Input Validation**: Sanitize all user-provided server metadata

### Data Protection

- **Encryption**: TLS encryption for all communications
- **Data Validation**: Validate server URLs and metadata
- **Audit Logging**: Log all registry modifications

### Network Security

- **Firewall Rules**: Restrict access to registry ports
- **TLS Configuration**: Use proper certificates in production
- **Private Registries**: Support for authenticated registry access

## Integration Examples

### JavaScript Client

```javascript
import { MCPRegistryClient } from 'mcp-registry-client';

const registry = new MCPRegistryClient({
  endpoint: 'http://localhost:8913'
});

// Search for servers
const servers = await registry.browse({
  q: 'weather',
  transport: 'http'
});

// Register a new server
await registry.upload({
  name: 'My Weather Server',
  packages: [{
    name: 'weather-tools',
    transport: { type: 'http', url: 'http://my-server:8080' }
  }]
});
```

### Python Client

```python
from mcp_registry import RegistryClient

client = RegistryClient("http://localhost:8913")

# List all servers
servers = client.browse()

# Filter by transport
http_servers = client.browse(transport="http")

# Upload custom server
client.upload({
    "name": "Custom Server",
    "packages": [{
        "name": "custom-tools",
        "transport": {"type": "http", "url": "http://localhost:8080"}
    }]
})
```

### cURL Examples

```bash
# List all servers
curl http://localhost:8913/api/v1/registry/browse

# Search for servers
curl "http://localhost:8913/api/v1/registry/browse?q=weather"

# Upload a server
curl -X POST http://localhost:8913/api/v1/registry/upload \
  -H "Content-Type: application/json" \
  -d '{"name":"Test Server","packages":[{"name":"test","transport":{"type":"http","url":"http://test:8080"}}]}'

# Get server details
curl http://localhost:8913/api/v1/registry/server-123
```

## Migration Guide

### From Local Server Lists

**Before:**
```
servers.json
├── server1: {url: "http://server1:8080"}
├── server2: {url: "http://server2:8080"}
```

**After:**
```bash
# Bulk upload to registry
curl -X POST http://localhost:8913/api/v1/registry/upload/bulk \
  -H "Content-Type: application/json" \
  -d @servers.json
```

### Configuration Migration

1. **Update Client Code**: Change from local lists to registry API calls
2. **Enable Sync Sources**: Configure official and Docker registry sync
3. **Set Up Authentication**: Add authentication for registry access
4. **Test Integration**: Verify all servers are accessible through registry

### Compatibility Matrix

| Feature | Version | Status |
|---------|---------|--------|
| Server Upload | 1.0.0 | ✅ Full |
| Bulk Upload | 1.0.0 | ✅ Full |
| Official Sync | 1.0.0 | ✅ Full |
| Docker Sync | 1.0.0 | ✅ Full |
| Search API | 1.0.0 | ✅ Full |
| Validation | 1.0.0 | ⚠️ Basic |

## Advanced Configuration

### Custom Sync Sources

```go
// Add custom registry source
registry.AddSyncSource("custom", &CustomSyncSource{
    URL: "https://custom-registry.example.com",
    Auth: &AuthConfig{Token: "token"},
})
```

### Custom Validation Rules

```go
// Add custom validation
registry.AddValidator("custom", func(server *models.MCPServer) error {
    // Custom validation logic
    return nil
})
```

### Plugin Integration

```go
// Load registry plugins
registry.LoadPlugin("validation", &CustomValidationPlugin{})
registry.LoadPlugin("sync", &CustomSyncPlugin{})
```

This comprehensive registry service provides a robust, scalable, and extensible catalog system for MCP servers while maintaining high performance and reliability.</content>
<parameter name="filePath">docs/services/registry.md