# Getting Started with SUSE AI Universal Proxy

This guide will help you get up and running with SUSE AI Universal Proxy quickly. Whether you're evaluating the system or deploying it in production, follow these steps to get started.

## Prerequisites

Before you begin, ensure you have the following:

- **Go 1.21+** (for building from source)
- **Docker** (for containerized deployment)
- **Kubernetes cluster** (for production deployment)
- **Helm 3.0+** (for Kubernetes deployment)

## Quick Start Options

### 🚀 Option 1: Docker (Fastest)

**Run the complete system:**
```bash
# Pull and run all services
docker run -d \
  --name suse-ai-up \
  -p 8080:8080 \
  -p 8911:8911 \
  -p 8912:8912 \
  -p 8913:8913 \
  -p 8914:8914 \
  -p 38080:38080 \
  -p 38912:38912 \
  -p 38913:38913 \
  -p 38914:38914 \
  -p 3911:3911 \
  suse/suse-ai-up:latest

# Check health
curl http://localhost:8911/health
```

**Run individual services:**
```bash
# Just the proxy service
docker run -d -p 8080:8080 suse/suse-ai-up:latest ./suse-ai-up proxy

# Proxy + Registry
docker run -d \
  -p 8080:8080 \
  -p 8913:8913 \
  suse/suse-ai-up:latest ./suse-ai-up proxy registry
```

### ☸️ Option 2: Kubernetes + Helm (Recommended)

**Add the Helm repository:**
```bash
# Add SUSE charts repository
helm repo add suse https://charts.suse.com
helm repo update

# Install with default configuration
helm install suse-ai-up suse/suse-ai-up
```

**Custom configuration:**
```yaml
# values.yaml
services:
  proxy:
    enabled: true
  registry:
    enabled: true
  discovery:
    enabled: true
  plugins:
    enabled: true

tls:
  enabled: true
  autoGenerate: true

monitoring:
  enabled: false
```

```bash
# Install with custom values
helm install suse-ai-up suse/suse-ai-up -f values.yaml
```

### 🛠️ Option 3: Build from Source

**Clone and build:**
```bash
git clone https://github.com/suse/suse-ai-up.git
cd suse-ai-up

# Build the binary
go build -o suse-ai-up ./cmd

# Run all services
./suse-ai-up all
```

## First Steps

### 1. Verify Installation

**Check service health:**
```bash
# Unified health check
curl http://localhost:8911/health

# Expected response:
{
  "status": "healthy",
  "timestamp": "2025-12-04T12:00:00Z",
  "services": {
    "proxy": "healthy",
    "registry": "healthy",
    "discovery": "healthy",
    "plugins": "healthy"
  }
}
```

### 2. Access API Documentation

**Open Swagger UI:**
```bash
# In your browser
open http://localhost:8911/docs
```

This provides interactive API documentation for all services.

### 3. Basic MCP Proxy Usage

**Test the proxy with a simple MCP server:**
```bash
# Example: Connect to an MCP-compatible AI service
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {},
      "clientInfo": {
        "name": "test-client",
        "version": "1.0"
      }
    }
  }'
```

## Configuration Examples

### Environment Variables

```bash
# TLS Configuration
export TLS_CERT_FILE=/path/to/cert.pem
export TLS_KEY_FILE=/path/to/key.pem
export AUTO_TLS=true

# Service Ports
export PROXY_PORT=8080
export REGISTRY_PORT=8913
export DISCOVERY_PORT=8912
export PLUGINS_PORT=8914

# Run with custom config
./suse-ai-up all
```

### Docker Compose

```yaml
# docker-compose.yml
version: '3.8'
services:
  suse-ai-up:
    image: suse/suse-ai-up:latest
    ports:
      - "8080:8080"    # Proxy HTTP
      - "8911:8911"    # Health/Docs HTTP
      - "8912:8912"    # Discovery HTTP
      - "8913:8913"    # Registry HTTP
      - "8914:8914"    # Plugins HTTP
      - "38080:38080"  # Proxy HTTPS
      - "38912:38912"  # Discovery HTTPS
      - "38913:38913"  # Registry HTTPS
      - "38914:38914"  # Plugins HTTPS
      - "3911:3911"    # Health/Docs HTTPS
    environment:
      - AUTO_TLS=true
    command: ["./suse-ai-up", "all"]
```

### Kubernetes Manifest

```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: suse-ai-up
spec:
  replicas: 1
  selector:
    matchLabels:
      app: suse-ai-up
  template:
    metadata:
      labels:
        app: suse-ai-up
    spec:
      containers:
      - name: proxy
        image: suse/suse-ai-up:latest
        command: ["./suse-ai-up", "proxy"]
        ports:
        - containerPort: 8080
          name: proxy-http
        - containerPort: 38080
          name: proxy-https
        env:
        - name: AUTO_TLS
          value: "true"
      - name: registry
        image: suse/suse-ai-up:latest
        command: ["./suse-ai-up", "registry"]
        ports:
        - containerPort: 8913
          name: registry-http
        - containerPort: 38913
          name: registry-https
      - name: discovery
        image: suse/suse-ai-up:latest
        command: ["./suse-ai-up", "discovery"]
        ports:
        - containerPort: 8912
          name: discovery-http
        - containerPort: 38912
          name: discovery-https
      - name: plugins
        image: suse/suse-ai-up:latest
        command: ["./suse-ai-up", "plugins"]
        ports:
        - containerPort: 8914
          name: plugins-http
        - containerPort: 38914
          name: plugins-https
---
apiVersion: v1
kind: Service
metadata:
  name: suse-ai-up
spec:
  selector:
    app: suse-ai-up
  ports:
  - name: proxy-http
    port: 8080
    targetPort: 8080
  - name: proxy-https
    port: 38080
    targetPort: 38080
  - name: registry-http
    port: 8913
    targetPort: 8913
  - name: registry-https
    port: 38913
    targetPort: 38913
  - name: discovery-http
    port: 8912
    targetPort: 8912
  - name: discovery-https
    port: 38912
    targetPort: 38912
  - name: plugins-http
    port: 8914
    targetPort: 8914
  - name: plugins-https
    port: 38914
    targetPort: 38914
  - name: health-http
    port: 8911
    targetPort: 8911
  - name: health-https
    port: 3911
    targetPort: 3911
  type: LoadBalancer
```

## Next Steps

### Explore the Services

1. **📚 Registry Service**
   ```bash
   # Browse available MCP servers
   curl http://localhost:8913/api/v1/registry/browse

   # Sync from official registry
   curl -X POST http://localhost:8913/api/v1/registry/sync/official
   ```

2. **🔍 Discovery Service**
   ```bash
   # Start a network scan
   curl -X POST http://localhost:8912/api/v1/scan \
     -H "Content-Type: application/json" \
     -d '{"scanRanges": ["192.168.1.0/24"], "ports": ["8080-8100"]}'

   # Check scan status
   curl http://localhost:8912/api/v1/scan/scan-123
   ```

3. **🔌 Plugins Service**
   ```bash
   # List registered plugins
   curl http://localhost:8914/api/v1/plugins

   # Register a new plugin
   curl -X POST http://localhost:8914/api/v1/plugins/register \
     -H "Content-Type: application/json" \
     -d '{"service_id": "my-plugin", "service_type": "smartagents"}'
   ```

### Advanced Configuration

- **[Helm Chart Configuration](docs/deployment/helm.md)** - Detailed Helm setup
- **[Security Configuration](docs/security.md)** - TLS, authentication, and security
- **[Monitoring Setup](docs/deployment/kubernetes.md)** - Prometheus and Grafana integration

### Development

- **[API Reference](docs/api-reference.md)** - Complete API documentation
- **[Plugin Development](docs/services/plugins.md)** - Create custom plugins
- **[Contributing Guide](CONTRIBUTING.md)** - Development workflow

## Troubleshooting

### Common Issues

**Services not starting:**
```bash
# Check logs
docker logs suse-ai-up

# Verify ports are available
netstat -tlnp | grep :8080
```

**TLS certificate errors:**
```bash
# Use -k flag to ignore certificate validation
curl -k https://localhost:38080/health

# Or configure proper certificates
export TLS_CERT_FILE=/path/to/cert.pem
export TLS_KEY_FILE=/path/to/key.pem
```

**Port conflicts:**
```bash
# Change default ports
export PROXY_PORT=8081
export REGISTRY_PORT=8915
```

### Getting Help

- **📖 Documentation**: [docs/](docs/)
- **🐛 Issues**: [GitHub Issues](https://github.com/suse/suse-ai-up/issues)
- **💬 Discussions**: [GitHub Discussions](https://github.com/suse/suse-ai-up/discussions)

---

**Ready to explore?** Check out the [service documentation](docs/services/) to learn about each component's capabilities, or dive into the [API reference](docs/api-reference.md) for detailed endpoint information.