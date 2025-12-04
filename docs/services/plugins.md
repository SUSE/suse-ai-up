# MCP Plugins Service

The MCP Plugins Service manages the lifecycle, health monitoring, and orchestration of MCP (Model Context Protocol) plugins within the SUSE AI Universal Proxy system. It provides a centralized platform for plugin registration, execution monitoring, and dynamic loading capabilities.

## Overview

The plugins service acts as a plugin orchestrator that:

- **Plugin Management**: Registers and manages MCP plugin instances
- **Health Monitoring**: Continuously monitors plugin health and performance
- **Dynamic Loading**: Supports runtime plugin loading and unloading
- **Service Integration**: Coordinates with registry and discovery services
- **Lifecycle Management**: Handles plugin startup, shutdown, and restarts

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
│   PROXY     │◄──►│  PLUGINS    │
│  (Primary)  │    │ (Sidecar)   │
│             │    │             │
│ Port: 8080  │    │ Port: 8914  │
│ HTTPS:38080 │    │ HTTPS:38914 │
└──────┬──────┘    └─────────────┘
       │
       ▼
┌─────────────┐
│ MCP PLUGIN  │
│ (Managed)   │
└─────────────┘
```

### Key Components

- **Plugin Manager**: Core orchestration engine for plugin lifecycle
- **Health Monitor**: Continuous health checking and status tracking
- **Registry Integration**: Automatic plugin registration with central registry
- **Service Coordinator**: Manages inter-plugin communication and dependencies
- **Plugin Loader**: Dynamic loading and unloading of plugin modules

## Configuration

### Environment Variables

```bash
# Basic Configuration
PLUGINS_PORT=8914              # HTTP port (default: 8914)
TLS_PORT=38914                 # HTTPS port (default: 38914)

# TLS Configuration
AUTO_TLS=true                  # Auto-generate self-signed certificates
TLS_CERT_FILE=/path/to/cert.pem
TLS_KEY_FILE=/path/to/key.pem

# Health Monitoring
HEALTH_INTERVAL=30s            # Health check interval (default: 30s)
HEALTH_TIMEOUT=5s              # Health check timeout
MAX_FAILURES=3                 # Max failures before restart

# Plugin Management
MAX_PLUGINS=50                 # Maximum concurrent plugins
PLUGIN_TIMEOUT=60s             # Plugin operation timeout
AUTO_RESTART=true              # Auto-restart failed plugins
```

### Docker Configuration

```yaml
services:
  plugins:
    image: suse/suse-ai-up:latest
    ports:
      - "8914:8914"      # HTTP
      - "38914:38914"    # HTTPS
    environment:
      - AUTO_TLS=true
      - HEALTH_INTERVAL=30s
      - MAX_PLUGINS=50
    command: ["./suse-ai-up", "plugins"]
```

### Kubernetes Configuration

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: suse-ai-up-plugins
spec:
  template:
    spec:
      containers:
      - name: plugins
        image: suse/suse-ai-up:latest
        ports:
        - containerPort: 8914
          name: http
        - containerPort: 38914
          name: https
        env:
        - name: AUTO_TLS
          value: "true"
        - name: HEALTH_INTERVAL
          value: "30s"
        command: ["./suse-ai-up", "plugins"]
```

## API Endpoints

### Plugin Registration

#### Register Plugin
```http
POST /api/v1/plugins/register
Content-Type: application/json

{
  "name": "weather-plugin",
  "version": "1.0.0",
  "description": "Weather data and forecasting plugin",
  "type": "mcp-server",
  "endpoint": {
    "type": "http",
    "url": "http://weather-plugin:8080",
    "healthCheck": "/health"
  },
  "capabilities": {
    "tools": ["get_weather", "forecast"],
    "resources": ["weather-data"],
    "prompts": []
  },
  "metadata": {
    "author": "SUSE",
    "license": "MIT",
    "tags": ["weather", "api"]
  }
}
```

Response:
```json
{
  "pluginId": "plugin-1733313600123456789",
  "status": "registered",
  "registrationTime": "2025-12-04T12:00:00Z",
  "healthStatus": "checking"
}
```

#### List Plugins
```http
GET /api/v1/plugins
```

Query Parameters:
- `status`: Filter by status (active, inactive, failed, registering)
- `type`: Filter by plugin type (mcp-server, adapter, middleware)
- `name`: Filter by plugin name (partial match)

Response:
```json
{
  "plugins": [
    {
      "pluginId": "plugin-1733313600123456789",
      "name": "weather-plugin",
      "version": "1.0.0",
      "status": "active",
      "type": "mcp-server",
      "endpoint": {
        "type": "http",
        "url": "http://weather-plugin:8080"
      },
      "capabilities": {
        "tools": ["get_weather", "forecast"],
        "resources": ["weather-data"]
      },
      "health": {
        "status": "healthy",
        "lastCheck": "2025-12-04T12:00:30Z",
        "responseTime": 45,
        "consecutiveFailures": 0
      },
      "registrationTime": "2025-12-04T12:00:00Z",
      "metadata": {
        "author": "SUSE",
        "tags": ["weather", "api"]
      }
    }
  ],
  "totalCount": 5,
  "activeCount": 4
}
```

#### Get Plugin by ID
```http
GET /api/v1/plugins/{plugin_id}
```

Response:
```json
{
  "pluginId": "plugin-1733313600123456789",
  "name": "weather-plugin",
  "status": "active",
  "health": {
    "status": "healthy",
    "lastCheck": "2025-12-04T12:00:30Z",
    "responseTime": 45,
    "uptime": "2h30m",
    "consecutiveFailures": 0
  },
  "metrics": {
    "requestsTotal": 1250,
    "requestsPerSecond": 12.5,
    "errorRate": 0.02,
    "averageResponseTime": 45
  },
  "configuration": {...},
  "logs": [...]
}
```

#### Update Plugin
```http
PUT /api/v1/plugins/{plugin_id}
Content-Type: application/json

{
  "status": "inactive",
  "configuration": {
    "maxConnections": 100,
    "timeout": "30s"
  }
}
```

#### Unregister Plugin
```http
DELETE /api/v1/plugins/{plugin_id}
```

### Health Monitoring

#### Get Plugin Health
```http
GET /api/v1/health/{plugin_id}
```

Response:
```json
{
  "pluginId": "plugin-1733313600123456789",
  "status": "healthy",
  "lastCheck": "2025-12-04T12:00:30Z",
  "responseTime": 45,
  "uptime": "2h30m",
  "consecutiveFailures": 0,
  "checks": [
    {
      "type": "http",
      "endpoint": "/health",
      "status": "pass",
      "responseTime": 45,
      "timestamp": "2025-12-04T12:00:30Z"
    },
    {
      "type": "mcp",
      "method": "initialize",
      "status": "pass",
      "timestamp": "2025-12-04T12:00:30Z"
    }
  ]
}
```

#### Bulk Health Check
```http
GET /api/v1/health
```

Response:
```json
{
  "summary": {
    "totalPlugins": 5,
    "healthy": 4,
    "unhealthy": 1,
    "unknown": 0
  },
  "plugins": [
    {
      "pluginId": "plugin-1733313600123456789",
      "name": "weather-plugin",
      "status": "healthy",
      "lastCheck": "2025-12-04T12:00:30Z"
    }
  ]
}
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
  "service": "plugins",
  "timestamp": "2025-12-04T12:00:00Z",
  "activePlugins": 4,
  "totalPlugins": 5
}
```

## Plugin Lifecycle

### Registration Process

1. **Plugin Submission**: Client submits plugin configuration
2. **Validation**: Service validates plugin metadata and endpoint
3. **Health Check**: Initial health assessment
4. **Registration**: Plugin added to active registry
5. **Monitoring Start**: Continuous health monitoring begins

### Plugin States

- **registering**: Plugin being registered and validated
- **active**: Plugin is running and healthy
- **inactive**: Plugin temporarily disabled
- **failed**: Plugin has failed health checks
- **unregistering**: Plugin being removed

### Health Monitoring

The plugins service performs comprehensive health monitoring:

```yaml
healthChecks:
  - type: "http"
    endpoint: "/health"
    interval: 30s
    timeout: 5s
    failureThreshold: 3
  - type: "mcp"
    method: "initialize"
    interval: 60s
    timeout: 10s
    failureThreshold: 2
  - type: "custom"
    command: "health_check.sh"
    interval: 120s
    timeout: 15s
```

### Automatic Recovery

Failed plugins can be automatically recovered:

```yaml
recovery:
  enabled: true
  strategies:
    - type: "restart"
      maxAttempts: 3
      backoff: "30s"
    - type: "failover"
      backupEndpoint: "http://backup-plugin:8080"
    - type: "notification"
      webhook: "https://alerts.example.com/plugin-failed"
```

## Plugin Types

### MCP Server Plugins

Standard MCP servers providing tools, resources, and prompts:

```json
{
  "type": "mcp-server",
  "capabilities": {
    "tools": ["tool1", "tool2"],
    "resources": ["resource1"],
    "prompts": ["prompt1"]
  },
  "endpoint": {
    "type": "http",
    "url": "http://mcp-server:8080"
  }
}
```

### Adapter Plugins

Protocol adapters for different MCP transport types:

```json
{
  "type": "adapter",
  "adapts": {
    "from": "websocket",
    "to": "http"
  },
  "configuration": {
    "bufferSize": 1024,
    "timeout": "30s"
  }
}
```

### Middleware Plugins

Request/response processing middleware:

```json
{
  "type": "middleware",
  "hooks": ["pre-request", "post-response"],
  "configuration": {
    "logging": true,
    "caching": false
  }
}
```

## Plugin Metadata

### Plugin Object Structure

```json
{
  "pluginId": "unique-plugin-id",
  "name": "plugin-display-name",
  "version": "1.2.3",
  "description": "Detailed plugin description",
  "type": "mcp-server|adapter|middleware",
  "status": "active|inactive|failed|registering",
  "endpoint": {
    "type": "http|websocket|sse",
    "url": "endpoint-url",
    "healthCheck": "/health",
    "auth": {
      "type": "bearer",
      "token": "auth-token"
    }
  },
  "capabilities": {
    "tools": ["tool1", "tool2"],
    "resources": ["resource1"],
    "prompts": ["prompt1"],
    "adapters": ["websocket", "sse"]
  },
  "configuration": {
    "maxConnections": 100,
    "timeout": "30s",
    "retryPolicy": "exponential"
  },
  "health": {
    "status": "healthy|unhealthy|unknown",
    "lastCheck": "2025-12-04T12:00:00Z",
    "responseTime": 45,
    "uptime": "2h30m",
    "consecutiveFailures": 0
  },
  "metrics": {
    "requestsTotal": 1250,
    "requestsPerSecond": 12.5,
    "errorRate": 0.02,
    "averageResponseTime": 45
  },
  "registrationTime": "2025-12-04T10:00:00Z",
  "lastUpdated": "2025-12-04T12:00:00Z",
  "metadata": {
    "author": "SUSE",
    "license": "MIT",
    "tags": ["weather", "api"],
    "documentation": "https://docs.example.com/plugin"
  }
}
```

## Performance Monitoring

### Metrics Collection

The plugins service collects comprehensive metrics:

```
# Plugin metrics
mcp_plugins_active_total 4
mcp_plugins_failed_total 1

# Health check metrics
mcp_plugins_health_checks_total{result="success"} 240
mcp_plugins_health_checks_total{result="failure"} 6

# Performance metrics
mcp_plugins_request_duration_seconds{plugin="weather-plugin", quantile="0.95"} 0.045
mcp_plugins_requests_total{plugin="weather-plugin", method="tools/call"} 1250

# Resource metrics
mcp_plugins_memory_usage_bytes{plugin="weather-plugin"} 67108864
mcp_plugins_cpu_usage_percent{plugin="weather-plugin"} 12.5
```

### Performance Tuning

```bash
export HEALTH_CHECK_INTERVAL=30s
export MAX_CONCURRENT_CHECKS=10
export METRICS_RETENTION=7d
export ALERT_THRESHOLD=0.05  # 5% error rate
```

## Security Considerations

### Plugin Authentication

- **Endpoint Security**: Secure communication with plugin endpoints
- **Token Management**: Secure storage and rotation of auth tokens
- **Access Control**: Plugin-specific access permissions

### Health Check Security

- **Safe Endpoints**: Health checks don't expose sensitive information
- **Rate Limiting**: Prevent health check abuse
- **Timeout Protection**: Prevent hanging health checks

### Plugin Isolation

- **Resource Limits**: CPU and memory limits per plugin
- **Network Isolation**: Plugin network access restrictions
- **Failure Containment**: Plugin failures don't affect other plugins

## Troubleshooting

### Common Issues

**Plugin Registration Failures**
```bash
# Check plugin configuration
curl http://localhost:8914/api/v1/plugins/plugin-123

# Validate endpoint accessibility
curl http://plugin-endpoint:8080/health

# Check service logs
kubectl logs deployment/suse-ai-up-plugins
```

**Health Check Failures**
```bash
# Get detailed health status
curl http://localhost:8914/api/v1/health/plugin-123

# Test manual health check
curl http://plugin-endpoint:8080/health

# Check network connectivity
telnet plugin-endpoint 8080
```

**Performance Issues**
```bash
# Check plugin metrics
curl http://localhost:8914/api/v1/plugins/plugin-123

# Monitor resource usage
kubectl top pods

# Check for bottlenecks
kubectl describe pod plugin-pod
```

### Debug Mode

Enable detailed logging for troubleshooting:

```bash
export LOG_LEVEL=debug
export LOG_FORMAT=json
./suse-ai-up plugins
```

### Plugin Recovery

Manually recover failed plugins:

```bash
# Restart plugin
curl -X PUT http://localhost:8914/api/v1/plugins/plugin-123 \
  -d '{"action": "restart"}'

# Re-register plugin
curl -X POST http://localhost:8914/api/v1/plugins/register \
  -d @plugin-config.json
```

## Integration Examples

### JavaScript Client

```javascript
import { MCPPluginsClient } from 'mcp-plugins-client';

const plugins = new MCPPluginsClient({
  endpoint: 'http://localhost:8914'
});

// Register a new plugin
const plugin = await plugins.register({
  name: 'weather-plugin',
  type: 'mcp-server',
  endpoint: { type: 'http', url: 'http://weather:8080' },
  capabilities: { tools: ['get_weather'] }
});

// Monitor plugin health
const health = await plugins.getHealth(plugin.pluginId);

// List all plugins
const allPlugins = await plugins.listPlugins({ status: 'active' });
```

### Python Client

```python
from mcp_plugins import PluginsClient

client = PluginsClient("http://localhost:8914")

# Register plugin
plugin = client.register({
    "name": "weather-plugin",
    "type": "mcp-server",
    "endpoint": {"type": "http", "url": "http://weather:8080"},
    "capabilities": {"tools": ["get_weather"]}
})

# Check health
health = client.get_health(plugin["pluginId"])

# List plugins
plugins = client.list_plugins(status="active")
```

### cURL Examples

```bash
# Register plugin
curl -X POST http://localhost:8914/api/v1/plugins/register \
  -H "Content-Type: application/json" \
  -d '{
    "name": "weather-plugin",
    "type": "mcp-server",
    "endpoint": {"type": "http", "url": "http://weather:8080"}
  }'

# List plugins
curl http://localhost:8914/api/v1/plugins

# Get plugin health
curl http://localhost:8914/api/v1/health/plugin-123

# Update plugin
curl -X PUT http://localhost:8914/api/v1/plugins/plugin-123 \
  -d '{"status": "inactive"}'

# Unregister plugin
curl -X DELETE http://localhost:8914/api/v1/plugins/plugin-123
```

## Migration Guide

### From Static Configuration

**Before:**
```
plugins.json
├── weather: {url: "http://weather:8080"}
├── calculator: {url: "http://calc:8080"}
```

**After:**
```bash
# Register plugins dynamically
curl -X POST http://localhost:8914/api/v1/plugins/register \
  -d @plugins.json

# Plugins are now managed and monitored
```

### Configuration Migration

1. **Plugin Inventory**: Identify all existing plugins
2. **Configuration Export**: Convert static configs to API format
3. **Bulk Registration**: Register all plugins with the service
4. **Health Setup**: Configure appropriate health checks
5. **Monitoring**: Set up alerting and monitoring

### Compatibility Matrix

| Feature | Version | Status |
|---------|---------|--------|
| Plugin Registration | 1.0.0 | ✅ Full |
| Health Monitoring | 1.0.0 | ✅ Full |
| Dynamic Loading | 1.0.0 | ✅ Full |
| Metrics Collection | 1.0.0 | ✅ Full |
| Auto Recovery | 1.0.0 | ⚠️ Basic |

## Advanced Configuration

### Custom Health Checks

```go
// Add custom health check
plugin.AddHealthCheck("custom", &CustomHealthCheck{
    Command: "health_check.sh",
    Timeout: 10 * time.Second,
    Interval: 30 * time.Second,
})
```

### Plugin Dependencies

```go
// Define plugin dependencies
plugin.SetDependencies([]string{"database-plugin", "cache-plugin"})
```

### Custom Metrics

```go
// Add custom metrics
plugin.AddMetric("custom_metric", func() float64 {
    // Custom metric calculation
    return calculateCustomMetric()
})
```

This comprehensive plugins service provides robust lifecycle management, health monitoring, and orchestration capabilities for MCP plugins within the SUSE AI Universal Proxy ecosystem.</content>
<parameter name="filePath">docs/services/plugins.md