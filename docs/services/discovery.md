# MCP Discovery Service

The MCP Discovery Service provides automated network scanning and server discovery capabilities for the SUSE AI Universal Proxy system. It actively scans networks and environments to find MCP servers, validate their functionality, and register them with the central registry.

## Overview

The discovery service acts as an intelligent network scanner that:

- **Network Scanning**: Automated discovery of MCP servers on networks
- **Server Validation**: Tests discovered servers for MCP compliance
- **Registry Integration**: Automatically registers validated servers
- **Health Monitoring**: Continuous validation of discovered servers
- **Multi-Protocol Support**: Discovers servers using various transport protocols

## Architecture

### Service Position
```
┌─────────────┐
│   NETWORK   │
│  (Local,    │
│   Cloud)    │
└──────┬──────┘
       │
       ▼
┌─────────────┐    ┌─────────────┐
│ DISCOVERY   │◄──►│  REGISTRY   │
│  (Scanner)  │    │ (Catalog)   │
│             │    │             │
│ Port: 8912  │    │ Port: 8913  │
│ HTTPS:38912 │    │ HTTPS:38913 │
└──────┬──────┘    └─────────────┘
       │
       ▼
┌─────────────┐
│ MCP SERVER  │
│ (Discovered)│
└─────────────┘
```

### Key Components

- **Network Scanner**: Core scanning engine for network discovery
- **Protocol Detectors**: Identify MCP servers by protocol signatures
- **Validation Engine**: Tests server functionality and compliance
- **Result Store**: Caches scan results and discovered servers
- **Health Checker**: Monitors discovered server health

## Configuration

### Environment Variables

```bash
# Basic Configuration
DISCOVERY_PORT=8912              # HTTP port (default: 8912)
TLS_PORT=38912                   # HTTPS port (default: 38912)

# TLS Configuration
AUTO_TLS=true                    # Auto-generate self-signed certificates
TLS_CERT_FILE=/path/to/cert.pem
TLS_KEY_FILE=/path/to/key.pem

# Scanning Configuration
DEFAULT_TIMEOUT=30s              # Default scan timeout
MAX_CONCURRENCY=10               # Maximum concurrent scans
EXCLUDE_PROXY=true               # Exclude proxy from scans

# Performance Tuning
SCAN_BATCH_SIZE=50               # Servers per scan batch
VALIDATION_TIMEOUT=10s           # Server validation timeout
HEALTH_CHECK_INTERVAL=60s        # Health check frequency
```

### Docker Configuration

```yaml
services:
  discovery:
    image: suse-ai-up:latest
    ports:
      - "8912:8912"      # HTTP
      - "38912:38912"    # HTTPS
    environment:
      - AUTO_TLS=true
      - DEFAULT_TIMEOUT=30s
      - MAX_CONCURRENCY=10
    command: ["./suse-ai-up", "discovery"]
```

### Kubernetes Configuration

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: suse-ai-up-discovery
spec:
  template:
    spec:
      containers:
      - name: discovery
        image: suse/suse-ai-up:latest
        ports:
        - containerPort: 8912
          name: http
        - containerPort: 38912
          name: https
        env:
        - name: AUTO_TLS
          value: "true"
        - name: DEFAULT_TIMEOUT
          value: "30s"
        command: ["./suse-ai-up", "discovery"]
```

## API Endpoints

### Scan Management

#### Start Network Scan
```http
POST /api/v1/scan
Content-Type: application/json

{
  "targetNetworks": ["192.168.1.0/24", "10.0.0.0/8"],
  "ports": [8080, 8911, 3000, 4000],
  "timeout": "30s",
  "maxConcurrent": 10,
  "excludeProxy": true,
  "protocols": ["http", "websocket"],
  "scanType": "comprehensive"
}
```

Response:
```json
{
  "scanId": "scan-1733313600123456789",
  "status": "started",
  "config": {
    "targetNetworks": ["192.168.1.0/24"],
    "ports": [8080, 8911],
    "timeout": "30s",
    "maxConcurrent": 10
  },
  "startTime": "2025-12-04T12:00:00Z"
}
```

#### Get Scan Status
```http
GET /api/v1/scan/{scan_id}
```

Response (Running):
```json
{
  "scanId": "scan-1733313600123456789",
  "status": "running",
  "startTime": "2025-12-04T12:00:00Z",
  "config": {...},
  "progress": {
    "networksScanned": 5,
    "totalNetworks": 10,
    "serversFound": 23
  }
}
```

Response (Completed):
```json
{
  "scanId": "scan-1733313600123456789",
  "status": "completed",
  "startTime": "2025-12-04T12:00:00Z",
  "endTime": "2025-12-04T12:05:30Z",
  "duration": "5m30s",
  "serverCount": 47,
  "results": [
    {
      "ip": "192.168.1.100",
      "port": 8080,
      "protocol": "http",
      "serverType": "mcp",
      "capabilities": ["tools", "resources"],
      "lastSeen": "2025-12-04T12:05:25Z"
    }
  ],
  "config": {...}
}
```

### Server Management

#### List All Discovered Servers
```http
GET /api/v1/servers
```

Query Parameters:
- `protocol`: Filter by protocol (http, websocket, sse)
- `status`: Filter by status (active, inactive, unknown)
- `since`: Filter by discovery time (RFC3339 timestamp)

Response:
```json
{
  "servers": [
    {
      "ip": "192.168.1.100",
      "port": 8080,
      "protocol": "http",
      "serverType": "mcp",
      "capabilities": ["tools", "resources", "prompts"],
      "status": "active",
      "firstSeen": "2025-12-04T10:00:00Z",
      "lastSeen": "2025-12-04T12:00:00Z",
      "scanId": "scan-1733313600123456789"
    }
  ],
  "totalCount": 47,
  "scanCount": 3
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
  "service": "discovery",
  "timestamp": "2025-12-04T12:00:00Z",
  "activeScans": 2,
  "totalServers": 156
}
```

## Scan Configuration

### Scan Types

#### Comprehensive Scan
Scans all specified networks and ports for MCP servers:

```json
{
  "scanType": "comprehensive",
  "targetNetworks": ["192.168.1.0/24", "10.0.0.0/8"],
  "ports": [8080, 8911, 3000, 4000, 5000],
  "protocols": ["http", "websocket", "sse"]
}
```

#### Targeted Scan
Scans specific IP ranges or individual hosts:

```json
{
  "scanType": "targeted",
  "targets": ["192.168.1.100", "mcp-server.example.com"],
  "ports": [8080],
  "protocols": ["http"]
}
```

#### Quick Scan
Fast scan of common MCP ports on local networks:

```json
{
  "scanType": "quick",
  "targetNetworks": ["192.168.1.0/24"],
  "ports": [8080, 8911],
  "timeout": "5s"
}
```

### Protocol Detection

The discovery service can detect MCP servers using multiple protocols:

- **HTTP**: Standard REST API endpoints
- **WebSocket**: Real-time bidirectional communication
- **Server-Sent Events**: Streaming responses
- **Custom Protocols**: Extensible protocol detection

### Server Validation

After discovery, servers are validated for MCP compliance:

1. **Protocol Handshake**: Verify MCP protocol support
2. **Capability Detection**: Identify available tools/resources
3. **Health Checks**: Ensure server responsiveness
4. **Metadata Collection**: Gather server information

## Discovery Process

### Scan Workflow

1. **Initialization**: Parse scan configuration and parameters
2. **Network Enumeration**: Generate target IP/port combinations
3. **Parallel Scanning**: Concurrent scanning with rate limiting
4. **Protocol Detection**: Identify potential MCP servers
5. **Validation**: Test discovered servers for MCP compliance
6. **Registration**: Add validated servers to registry
7. **Result Storage**: Save scan results for future reference

### Concurrency Control

```yaml
scanning:
  maxConcurrent: 10          # Maximum parallel scans
  batchSize: 50             # IPs per batch
  rateLimit: 100            # Requests per second
  timeout: 30s              # Per-server timeout
```

### Error Handling

- **Network Errors**: Retry with exponential backoff
- **Timeout Handling**: Configurable timeouts per operation
- **Rate Limiting**: Respect target server limits
- **Partial Failures**: Continue scanning despite individual failures

## Server Metadata

### Discovered Server Object

```json
{
  "ip": "192.168.1.100",
  "port": 8080,
  "hostname": "mcp-server-01",
  "protocol": "http",
  "serverType": "mcp",
  "version": "2024-11-05",
  "capabilities": {
    "tools": ["calculator", "weather", "search"],
    "resources": ["files", "database"],
    "prompts": ["code-review", "documentation"]
  },
  "status": "active",
  "responseTime": 45,
  "firstSeen": "2025-12-04T10:00:00Z",
  "lastSeen": "2025-12-04T12:00:00Z",
  "lastValidated": "2025-12-04T11:30:00Z",
  "validationStatus": "passed",
  "scanId": "scan-1733313600123456789",
  "metadata": {
    "os": "linux",
    "architecture": "amd64",
    "tags": ["production", "api"]
  }
}
```

### Status Values

- **active**: Server is responding and validated
- **inactive**: Server not responding to health checks
- **unknown**: Status not yet determined
- **validation_failed**: Server failed MCP validation

## Health Monitoring

### Continuous Validation

The discovery service continuously monitors discovered servers:

```yaml
healthMonitoring:
  enabled: true
  interval: 60s              # Check every minute
  timeout: 5s               # Health check timeout
  failureThreshold: 3       # Fail after 3 consecutive failures
  successThreshold: 1       # Recover after 1 success
```

### Health Check Types

- **Basic Connectivity**: TCP connection test
- **HTTP Health**: GET request to health endpoint
- **MCP Protocol**: Initialize request validation
- **Capability Verification**: Tool/resource availability

### Alerting Integration

Health status changes can trigger alerts:

```yaml
alerting:
  enabled: true
  endpoints:
    - type: "webhook"
      url: "https://alerts.example.com/webhook"
    - type: "slack"
      webhook: "https://hooks.slack.com/services/..."
```

## Performance Tuning

### Scanning Optimization

```bash
export SCAN_BATCH_SIZE=100
export MAX_CONCURRENT_SCANS=20
export SCAN_TIMEOUT=15s
export VALIDATION_TIMEOUT=5s
```

### Memory Management

```bash
export RESULT_CACHE_SIZE=10000
export SCAN_HISTORY_RETENTION=7d
export CLEANUP_INTERVAL=1h
```

### Network Optimization

```bash
export CONNECTION_POOL_SIZE=50
export CONNECTION_TIMEOUT=10s
export KEEP_ALIVE_DURATION=30s
```

## Monitoring & Observability

### Metrics

The discovery service exposes Prometheus-compatible metrics:

```
# Scan metrics
mcp_discovery_scans_total{status="completed"} 45
mcp_discovery_scan_duration_seconds{type="comprehensive"} 325.5

# Server discovery
mcp_discovery_servers_found_total 156
mcp_discovery_servers_active 142

# Health checks
mcp_discovery_health_checks_total{result="success"} 8940
mcp_discovery_health_checks_total{result="failure"} 156

# Performance
mcp_discovery_response_time_seconds{quantile="0.95"} 0.023
```

### Logging

Structured logging with configurable levels:

```json
{
  "timestamp": "2025-12-04T12:00:00Z",
  "level": "info",
  "service": "discovery",
  "event": "scan_completed",
  "scanId": "scan-1733313600123456789",
  "duration": "5m30s",
  "serversFound": 47
}
```

## Troubleshooting

### Common Issues

**Scan Not Finding Servers**
```bash
# Check network connectivity
ping 192.168.1.100

# Verify target networks
curl http://localhost:8912/api/v1/scan/scan-123

# Check firewall rules
sudo ufw status
```

**High Resource Usage**
```bash
# Reduce concurrency
export MAX_CONCURRENCY=5

# Increase timeouts
export SCAN_TIMEOUT=60s

# Monitor resource usage
kubectl top pods
```

**Validation Failures**
```bash
# Check server logs
kubectl logs deployment/mcp-server

# Test manual connection
curl http://192.168.1.100:8080/health

# Enable debug logging
export LOG_LEVEL=debug
```

### Debug Mode

Enable detailed logging for troubleshooting:

```bash
export LOG_LEVEL=debug
export LOG_FORMAT=json
./suse-ai-up discovery
```

### Scan Recovery

Restart interrupted scans:

```bash
# Get scan status
curl http://localhost:8912/api/v1/scan/scan-123

# Resume or restart scan
curl -X POST http://localhost:8912/api/v1/scan \
  -d '{"resumeScanId": "scan-123"}'
```

## Security Considerations

### Network Scanning

- **Permission Requirements**: Ensure proper network access permissions
- **Rate Limiting**: Respect target network policies
- **Scope Limitation**: Limit scanning to authorized networks
- **Audit Logging**: Log all scanning activities

### Data Protection

- **Sensitive Data**: Avoid scanning sensitive network segments
- **Encryption**: Use TLS for all communications
- **Access Control**: Restrict discovery service access
- **Data Retention**: Configure appropriate data retention policies

### Safe Scanning Practices

- **Target Validation**: Verify scanning targets are authorized
- **Impact Assessment**: Evaluate potential impact on target systems
- **Gradual Rollout**: Start with limited scope and expand gradually
- **Monitoring**: Monitor scanning impact on network performance

## Integration Examples

### JavaScript Client

```javascript
import { MCPDiscoveryClient } from 'mcp-discovery-client';

const discovery = new MCPDiscoveryClient({
  endpoint: 'http://localhost:8912'
});

// Start comprehensive scan
const scanResult = await discovery.startScan({
  targetNetworks: ['192.168.1.0/24'],
  ports: [8080, 8911],
  timeout: '30s'
});

// Monitor scan progress
const status = await discovery.getScanStatus(scanResult.scanId);

// List discovered servers
const servers = await discovery.listServers({
  status: 'active',
  protocol: 'http'
});
```

### Python Client

```python
from mcp_discovery import DiscoveryClient

client = DiscoveryClient("http://localhost:8912")

# Start network scan
scan = client.start_scan({
    "targetNetworks": ["192.168.1.0/24"],
    "ports": [8080, 8911]
})

# Check scan status
status = client.get_scan_status(scan["scanId"])

# Get discovered servers
servers = client.list_servers(protocol="http")
```

### cURL Examples

```bash
# Start network scan
curl -X POST http://localhost:8912/api/v1/scan \
  -H "Content-Type: application/json" \
  -d '{
    "targetNetworks": ["192.168.1.0/24"],
    "ports": [8080, 8911],
    "timeout": "30s"
  }'

# Check scan status
curl http://localhost:8912/api/v1/scan/scan-1733313600123456789

# List discovered servers
curl http://localhost:8912/api/v1/servers?status=active

# Health check
curl http://localhost:8912/health
```

## Migration Guide

### From Manual Discovery

**Before:**
```
# Manual server registration
servers.json
├── server1: {ip: "192.168.1.100", port: 8080}
├── server2: {ip: "192.168.1.101", port: 8080}
```

**After:**
```bash
# Run automated discovery
curl -X POST http://localhost:8912/api/v1/scan \
  -d '{"targetNetworks": ["192.168.1.0/24"], "ports": [8080]}'

# Discovery service finds and validates servers automatically
```

### Configuration Migration

1. **Network Configuration**: Define target networks and ports
2. **Schedule Scans**: Set up regular automated scanning
3. **Integration Setup**: Connect discovery to registry service
4. **Monitoring**: Configure alerting for discovery events

### Compatibility Matrix

| Feature | Version | Status |
|---------|---------|--------|
| Network Scanning | 1.0.0 | ✅ Full |
| Protocol Detection | 1.0.0 | ✅ Full |
| Server Validation | 1.0.0 | ✅ Full |
| Health Monitoring | 1.0.0 | ✅ Full |
| Registry Integration | 1.0.0 | ✅ Full |

## Advanced Configuration

### Custom Protocol Detectors

```go
// Add custom protocol detector
discovery.AddProtocolDetector("custom", &CustomDetector{
    Signature: "X-MCP-Protocol",
    Validator: validateCustomProtocol,
})
```

### Custom Validation Rules

```go
// Add custom validation
discovery.AddValidator("security", func(server *DiscoveredServer) error {
    // Custom security validation
    return nil
})
```

### Plugin Integration

```go
// Load discovery plugins
discovery.LoadPlugin("advanced-scan", &AdvancedScanPlugin{})
discovery.LoadPlugin("cloud-discovery", &CloudDiscoveryPlugin{})
```

This comprehensive discovery service provides automated, intelligent network scanning capabilities while maintaining security, performance, and reliability standards.</content>
<parameter name="filePath">docs/services/discovery.md