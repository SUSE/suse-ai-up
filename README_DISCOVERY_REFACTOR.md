# Discovery System Refactor

## Overview

The discovery system has been completely refactored to address performance, reusability, and functionality issues. The new architecture provides:

- **Scan Job Management**: Asynchronous scan operations with progress tracking
- **Persistent Storage**: Discovered servers are stored across API calls
- **Incremental Scanning**: Avoids rescanning recently checked addresses
- **Proper API Endpoints**: Complete REST API for scan management

## Architecture

### Core Components

1. **ScanManager**: Manages scan job lifecycle, tracks progress, and coordinates operations
2. **DiscoveryStore**: Persistent storage interface for discovered servers
3. **NetworkScanner**: Enhanced with incremental scanning and caching
4. **DiscoveryHandler**: Updated to use the new architecture

### Key Improvements

#### 1. Scan Job Management
```go
type ScanJob struct {
    ID          string
    Status      ScanStatus // pending, running, completed, failed, cancelled
    Config      ScanConfig
    Results     []DiscoveredServer
    Errors      []error
    StartTime   time.Time
    EndTime     *time.Time
    Progress    float64 // 0.0 to 1.0
}
```

#### 2. Persistent Discovery Store
```go
type DiscoveryStore interface {
    Save(server *DiscoveredServer) error
    GetAll() ([]DiscoveredServer, error)
    GetByID(id string) (*DiscoveredServer, error)
    UpdateLastSeen(id string, time.Time) error
    RemoveStale(threshold time.Duration) error
}
```

#### 3. Incremental Scanning
- Scan cache prevents rescanning addresses within a time window (default: 5 minutes)
- Dramatically improves performance for repeated scans
- Configurable cache duration

## API Endpoints

### Start Scan
```
POST /api/v1/discovery/scan
Content-Type: application/json

{
  "scanRanges": ["192.168.1.0/24"],
  "ports": ["8000", "8001", "8002"],
  "timeout": "30s",
  "maxConcurrent": 10
}

Response:
{
  "jobId": "scan-1",
  "status": "pending",
  "message": "Scan queued"
}
```

### List Scan Jobs
```
GET /api/v1/discovery/scan

Response:
[
  {
    "id": "scan-1",
    "status": "completed",
    "config": {...},
    "results": [...],
    "startTime": "2025-11-07T16:30:00Z",
    "progress": 1.0
  }
]
```

### Get Scan Job Status
```
GET /api/v1/discovery/scan/{jobId}

Response:
{
  "id": "scan-1",
  "status": "completed",
  "results": [
    {
      "id": "mcp-192.168.1.74-8002--mcp",
      "name": "MCP Example Server (No Auth)",
      "address": "http://192.168.1.74:8002",
      "protocol": "MCP",
      "connection": "SSE",
      "status": "discovered",
      "lastSeen": "2025-11-07T16:31:12Z",
      "metadata": {
        "auth_type": "none",
        "endpoint": "/mcp",
        "port": "8002"
      }
    }
  ]
}
```

### Cancel Scan Job
```
DELETE /api/v1/discovery/scan/{jobId}

Response:
{
  "message": "Scan job cancelled"
}
```

### List Discovered Servers
```
GET /api/v1/discovery/servers

Response:
{
  "servers": [
    {
      "id": "mcp-192.168.1.74-8002--mcp",
      "name": "MCP Example Server (No Auth)",
      "address": "http://192.168.1.74:8002",
      "protocol": "MCP",
      "connection": "SSE",
      "lastSeen": "2025-11-07T16:31:12Z"
    }
  ],
  "count": 1
}
```

### Get Specific Server
```
GET /api/v1/discovery/servers/{serverId}

Response: (same as above, single server object)
```

## Configuration Options

### ScanConfig
```go
type ScanConfig struct {
    ScanRanges       []string // IP ranges in CIDR notation, ranges, or single IPs
    Ports           []string // Port specifications (single ports, ranges, or lists)
    Timeout         string   // Scan timeout (e.g., "30s", "5m")
    MaxConcurrent   int      // Maximum concurrent scan operations
    ExcludeProxy    *bool    // Whether to exclude proxy addresses
    ExcludeAddresses []string // Additional addresses to skip
}
```

### Default Values
- **ScanRanges**: Auto-detected local network interfaces (/24 subnets)
- **Ports**: ["8000", "8001", "8002", "8003", "8004", "8080", "8888"]
- **Timeout**: "30s"
- **MaxConcurrent**: 10

## Migration Guide

### From Old API
The old synchronous scan API has been replaced with asynchronous job-based scanning:

**Old (Broken):**
```bash
curl -X POST /api/v1/discovery/scan -d '{"scanRanges": ["192.168.1.0/24"]}'
# Returns: immediate results (but empty due to bugs)
```

**New (Working):**
```bash
# Start scan
curl -X POST /api/v1/discovery/scan -d '{"scanRanges": ["192.168.1.0/24"]}'
# Returns: {"jobId": "scan-1", "status": "pending"}

# Check status
curl /api/v1/discovery/scan/scan-1
# Returns: job status and results when complete

# Get all discovered servers
curl /api/v1/discovery/servers
# Returns: all servers from persistent store
```

## Performance Improvements

1. **Incremental Scanning**: Avoids rescanning recently checked addresses
2. **Job-Based Processing**: Non-blocking scan operations
3. **Persistent Storage**: No need to rescan to get previous results
4. **Configurable Concurrency**: Optimized for different network sizes

## Testing

Run the integration test to verify the system works:
```bash
go test ./pkg/scanner/ -run TestDiscoveryIntegration -v
```

## Future Enhancements

1. **Background Scanning**: Scheduled periodic scans
2. **Scan Scheduling**: Cron-like scan scheduling
3. **Advanced Filtering**: Filter results by various criteria
4. **Scan History**: Keep historical scan results
5. **Distributed Scanning**: Coordinate scans across multiple nodes