# SUSE AI Universal Proxy - Overview

## Architecture Overview

SUSE AI Universal Proxy is a comprehensive, modular system designed to facilitate secure, scalable, and extensible AI model integrations through the Model Context Protocol (MCP). The system employs a **main container + sidecar architecture** where multiple specialized services run as coordinated containers within a single Kubernetes pod.

## Core Principles

### 🔄 **Modular Design**
- **Service Isolation**: Each service runs independently with clear boundaries
- **Dependency Management**: Services start in a specific order to ensure proper initialization
- **Resource Efficiency**: Shared pod resources with optimized container configurations

### 🔒 **Security First**
- **TLS Everywhere**: All services support HTTPS with automatic certificate generation
- **Authentication**: Multiple auth methods (OAuth, Bearer, API Keys, Basic Auth)
- **Network Security**: Service-to-service communication within pod boundaries

### 📈 **Production Ready**
- **Health Monitoring**: Comprehensive health checks and metrics
- **Scalability**: Horizontal pod scaling with proper resource management
- **Observability**: Integrated monitoring and logging capabilities

## Service Architecture

### Container Layout

```
┌─────────────────────────────────────────────────────────────┐
│                    SUSE AI Universal Proxy                  │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │
│  │   PROXY     │  │  REGISTRY   │  │ DISCOVERY   │          │
│  │  (Primary)  │  │  (Sidecar)  │  │  (Sidecar)  │          │
│  │             │  │             │  │             │          │
│  │ Port: 8080  │  │ Port: 8913  │  │ Port: 8912  │          │
│  │ HTTPS:38080 │  │ HTTPS:38913 │  │ HTTPS:38912 │          │
│  └─────────────┘  └─────────────┘  └─────────────┘          │
│                                                             │
│  ┌─────────────┐                                            │
│  │   PLUGINS   │                                            │
│  │  (Sidecar)  │                                            │
│  │             │                                            │
│  │ Port: 8914  │                                            │
│  │ HTTPS:38914 │                                            │
│  └─────────────┘                                            │
└─────────────────────────────────────────────────────────────┘
```

### Service Dependencies & Startup Order

Services are designed with clear dependencies and start in the following order:

1. **🔄 PROXY** (Primary Container)
   - **Purpose**: Main MCP proxy service
   - **Dependencies**: None (starts first)
   - **Provides**: MCP protocol proxying, session management

2. **📚 REGISTRY** (Sidecar)
   - **Purpose**: MCP server catalog and management
   - **Dependencies**: None (can start independently)
   - **Provides**: Server registration, search, validation

3. **🔍 DISCOVERY** (Sidecar)
   - **Purpose**: Network scanning and server detection
   - **Dependencies**: Registry (for storing discovered servers)
   - **Provides**: Network scanning, auth detection, vulnerability assessment

4. **🔌 PLUGINS** (Sidecar)
   - **Purpose**: Dynamic plugin management
   - **Dependencies**: Registry (for plugin discovery)
   - **Provides**: Plugin registration, health monitoring, routing

## Key Capabilities

### 🔄 MCP Protocol Support
- **Full JSON-RPC 2.0**: Complete protocol implementation
- **Multiple Transports**: HTTP, SSE, WebSocket support
- **Session Management**: Advanced session isolation and lifecycle
- **Protocol Translation**: Seamless conversion between transport types

### 🔍 Network Intelligence
- **CIDR Scanning**: Configurable IP range scanning
- **Protocol Detection**: Automatic MCP server identification
- **Authentication Analysis**: OAuth, Bearer, Basic, Digest detection
- **Security Assessment**: Vulnerability scoring and risk analysis

### 📚 Registry Management
- **Multi-Source**: Official MCP registry, Docker Hub, custom uploads
- **Search & Filter**: Advanced querying by transport, registry type, validation status
- **Validation**: Server capability verification and health checking
- **Sync Operations**: Automated registry synchronization

### 🔌 Plugin Ecosystem
- **Dynamic Loading**: Runtime plugin registration and management
- **Capability Routing**: Intelligent request routing based on capabilities
- **Health Monitoring**: Continuous plugin health assessment
- **Extension API**: Well-defined interfaces for custom plugins

## Deployment Models

### 🐳 **Docker (Development)**
```bash
# Single service
docker run -p 8080:8080 suse/suse-ai-up:latest ./suse-ai-up proxy

# Full stack
docker run -p 8080:8080 -p 8911-8914:8911-8914 suse/suse-ai-up:latest
```

### ☸️ **Kubernetes (Production)**
```yaml
# Helm deployment (recommended)
helm install suse-ai-up ./charts/suse-ai-up

# Service configuration
services:
  proxy:
    enabled: true
  registry:
    enabled: true
  discovery:
    enabled: true
  plugins:
    enabled: true
```

### 🏗️ **Helm Chart Features**
- **Service Enablement**: Individual service activation/deactivation
- **Resource Management**: Configurable CPU/memory limits
- **TLS Configuration**: Auto-generated or custom certificates
- **Monitoring Integration**: Optional Prometheus/Grafana deployment
- **Ingress Configuration**: Automatic ingress rule generation

## Security Architecture

### 🔐 **Authentication Methods**
- **OAuth 2.0**: Industry-standard authorization flows
- **Bearer Tokens**: JWT and custom token validation
- **API Keys**: Simple key-based authentication
- **Basic Authentication**: Username/password support

### 🛡️ **Network Security**
- **TLS Encryption**: End-to-end encryption for all services
- **Certificate Management**: Auto-generated or custom certificates
- **Service Isolation**: Pod-level network boundaries
- **Health Validation**: Continuous security posture assessment

### 📊 **Monitoring & Observability**
- **Health Endpoints**: Unified health checking across services
- **Metrics Collection**: Prometheus-compatible metrics
- **Structured Logging**: Consistent log format across services
- **Tracing Support**: Distributed tracing capabilities

## Configuration Management

### Environment Variables
```bash
# TLS Configuration
TLS_CERT_FILE=/path/to/cert.pem
TLS_KEY_FILE=/path/to/key.pem
AUTO_TLS=true

# Service Configuration
PROXY_PORT=8080
REGISTRY_PORT=8913
DISCOVERY_PORT=8912
PLUGINS_PORT=8914

# Monitoring
PROMETHEUS_URL=http://prometheus:9090
GRAFANA_URL=http://grafana:3000
```

### Helm Values
```yaml
# Service enablement
services:
  proxy:
    enabled: true
    resources:
      requests: {cpu: 100m, memory: 128Mi}
      limits: {cpu: 500m, memory: 512Mi}

# TLS configuration
tls:
  enabled: true
  autoGenerate: true
  certFile: ""
  keyFile: ""

# Monitoring
monitoring:
  enabled: false
  prometheus: ""
  grafana: ""
```

## Performance Characteristics

### 📈 **Scalability**
- **Horizontal Scaling**: Multiple pod replicas
- **Resource Efficiency**: Shared pod resources
- **Load Balancing**: Kubernetes service distribution
- **Auto-scaling**: HPA support for demand-based scaling

### ⚡ **Performance**
- **Low Latency**: Optimized for real-time MCP communication
- **Concurrent Processing**: Multi-threaded request handling
- **Caching**: Intelligent response caching
- **Connection Pooling**: Efficient resource utilization

## Integration Points

### 🤖 **AI/ML Platforms**
- **OpenAI**: Direct MCP protocol support
- **Anthropic**: Claude integration via MCP
- **Custom Models**: Extensible architecture for any MCP-compatible service

### ☁️ **Cloud Platforms**
- **Kubernetes**: Native container orchestration
- **Docker**: Containerized deployment
- **Helm**: Package management and deployment

### 📊 **Monitoring Stack**
- **Prometheus**: Metrics collection and alerting
- **Grafana**: Visualization and dashboards
- **ELK Stack**: Log aggregation and analysis

## Development & Extension

### 🛠️ **Plugin Development**
- **SDK**: Comprehensive plugin development kit
- **API**: Well-defined interfaces for custom plugins
- **Documentation**: Complete plugin development guide
- **Examples**: Sample plugins for common use cases

### 🔧 **API Integration**
- **REST APIs**: All services expose RESTful APIs
- **WebSocket Support**: Real-time communication channels
- **Event Streaming**: Server-sent events for live updates
- **Webhook Support**: External system integration

## Migration & Compatibility

### 📚 **Version Compatibility**
- **MCP Protocol**: Full compliance with latest specifications
- **API Stability**: Backward-compatible API evolution
- **Configuration**: Migration tools for configuration updates

### 🔄 **Upgrade Path**
- **Rolling Updates**: Zero-downtime service updates
- **Configuration Migration**: Automated config transformation
- **Data Migration**: Seamless data transfer between versions

This architecture provides a robust, scalable, and secure foundation for AI model integration while maintaining flexibility for future enhancements and customizations.