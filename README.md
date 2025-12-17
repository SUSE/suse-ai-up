# SUSE AI Uniproxy

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://golang.org)
[![Docker](https://img.shields.io/badge/Docker-multi--arch-blue.svg)](https://hub.docker.com)

A comprehensive, modular MCP (Model Context Protocol) proxy system that enables secure, scalable, and extensible AI model integrations.

## 🚀 Key Capabilities

**🔄 MCP Proxy Service** - Full-featured HTTP proxy for MCP servers with advanced session management, authentication, and protocol translation.

**🔍 Network Discovery** - Automated network scanning to discover MCP servers, detect authentication types, and assess security vulnerabilities.

**📚 Server Registry** - Curated registry of remote MCP servers from mcpservers.org, including GitHub, Notion, Sentry, Linear, and 20+ other popular services.

**🔌 Plugin Management** - Dynamic plugin system for extending functionality with service registration, health monitoring, and capability routing.

## 🏗️ Architecture

The system uses a **main container + sidecar architecture** where services run as coordinated containers within a single Kubernetes pod:

```
┌─────────────────────────────────────────────────────────────┐
│                    SUSE AI Universal Proxy                  │
│                                                             │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │                   UNIFIED SERVICE                       │ │
│  │                                                         │ │
│  │  • MCP Proxy with session management                   │ │
│  │  • Server registry and discovery                       │ │
│  │  • Plugin management and orchestration                 │ │
│  │  • Authentication and authorization                    │ │
│  │                                                         │ │
│  │              HTTP: 8911 | HTTPS: 3911                  │ │
│  └─────────────────────────────────────────────────────────┘ │
│                                                             │
│  ┌─────────────┐                                            │
│  │   PLUGINS   │                                            │
│  │  (External) │                                            │
│  │             │                                            │
│  │  Variable   │                                            │
│  │   Ports     │                                            │
│  └─────────────┘                                            │
└─────────────────────────────────────────────────────────────┘
```

## 🏃‍♂️ Quick Start

### Docker (Single Command)
```bash
docker run -p 8911:8911 suse/suse-ai-up:latest
```

### Kubernetes + Helm
```bash
helm repo add suse-ai-up https://charts.suse.com
helm install suse-ai-up suse-ai-up/suse-ai-up
```

### Local Development
```bash
git clone https://github.com/suse/suse-ai-up.git
cd suse-ai-up
go run ./cmd/uniproxy
```

## 📋 Service Overview

### 🔄 SUSE AI Universal Proxy
- **Purpose**: Unified MCP proxy service with integrated registry, discovery, and plugin management
- **Features**:
  - MCP protocol proxy with session management
  - Integrated server registry and catalog
  - Network discovery and automatic server detection
  - Plugin orchestration and lifecycle management
  - Authentication and authorization
  - TLS encryption support
- **Ports**: HTTP 8911, HTTPS 3911
- **Architecture**: Single unified service replacing separate microservices

### 🔌 External Plugins
- **Purpose**: Extensible plugin system for additional MCP server integrations
- **Features**: External plugin registration, health monitoring, custom MCP server types
- **Ports**: Variable (configured per plugin)
- **Integration**: Register with main proxy service via API

## 🌐 Remote MCP Servers

The registry includes **20+ curated remote MCP servers** from [mcpservers.org](https://mcpservers.org/remote-mcp-servers), providing instant access to popular services:

### 📊 Available Servers

| Service | Authentication | Category | Description |
|---------|----------------|----------|-------------|
| **GitHub** | OAuth | Development | Repository management, issues, PRs, code search |
| **Notion** | OAuth | Productivity | Document collaboration and knowledge base |
| **Sentry** | OAuth | Monitoring | Error tracking and performance monitoring |
| **Linear** | OAuth | Project Management | Issue tracking and agile workflows |
| **Figma** | OAuth | Design | Collaborative design and prototyping |
| **CoinGecko** | Open | Cryptocurrency | Market data and trading information |
| **Semgrep** | Open | Security | Code security and quality analysis |
| **Atlassian** | OAuth | Enterprise | Jira, Confluence, enterprise tools |

### 🚀 Quick Examples

#### GitHub MCP Server
```bash
# List repositories
curl -H "X-API-Key: dev-service-key-123" \
  "http://localhost:8911/api/v1/registry/browse?q=github"

# Create adapter for GitHub
curl -X POST http://localhost:8911/api/v1/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "github-adapter",
    "mcpServerId": "github",
    "authentication": {
      "type": "oauth",
      "oauth": {
        "clientId": "your-github-oauth-client-id",
        "clientSecret": "your-github-oauth-client-secret"
      }
    }
  }'
```

#### Atlassian MCP Server
```bash
# Browse Atlassian services
curl -H "X-API-Key: dev-service-key-123" \
  "http://localhost:8911/api/v1/registry/browse?q=atlassian"

# Configure Jira integration
curl -X POST http://localhost:8911/api/v1/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "jira-adapter",
    "mcpServerId": "atlassian",
    "authentication": {
      "type": "oauth",
      "oauth": {
        "clientId": "your-atlassian-oauth-client-id",
        "clientSecret": "your-atlassian-oauth-client-secret"
      }
    }
  }'
```

### 🔑 Authentication Setup

**OAuth Services**: Configure OAuth 2.0 credentials in your adapter configuration
**Open Services**: No authentication required - use directly

**Note**: Authentication is configured per-adapter, not in the registry. The registry only indicates which servers require user authentication.

### 🔄 Manual Registry Updates

To update the remote server list:
```bash
curl -X POST -H "X-API-Key: dev-service-key-123" \
  http://localhost:8911/api/v1/registry/reload
```

## 🔐 Authentication & Security

Supports multiple authentication methods:
- **OAuth 2.0** - Industry standard authorization
- **Bearer Tokens** - JWT and custom token support
- **API Keys** - Simple key-based authentication
- **Basic Auth** - Username/password authentication

**Documentation**: [Authentication Guide](docs/authentication.md)

## 📖 Documentation

- **[Getting Started](docs/getting-started.md)** - Complete setup guide
- **[API Reference](docs/api-reference.md)** - Complete API documentation
- **[Deployment Guide](docs/deployment/)** - Docker, Kubernetes, Helm
- **[Security](docs/security.md)** - Security considerations and best practices

## 🛠️ Deployment Options

### Docker
```bash
# Basic proxy only
docker run -p 8080:8080 suse/suse-ai-up:latest ./suse-ai-up proxy

# Full stack
docker run -p 8080:8080 -p 8911-8914:8911-8914 suse/suse-ai-up:latest
```

### Kubernetes
```bash
# Using Helm (recommended)
helm install suse-ai-up ./charts/suse-ai-up

# Using kubectl
kubectl apply -f examples/kubernetes/
```

### Helm Configuration
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
  autoGenerate: true  # Generates self-signed certs

monitoring:
  enabled: false  # Set to true to deploy Prometheus + Grafana
```

## 🔍 Health Checks & Monitoring

- **Unified Health Endpoint**: `http://localhost:8911/health`
- **API Documentation**: `http://localhost:8911/docs`
- **Prometheus Metrics**: Available when monitoring enabled
- **Grafana Dashboards**: Pre-configured dashboards included

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

## 📄 License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE.md) for details.

## 🆘 Support

- **Documentation**: [docs/](docs/)
- **Issues**: [GitHub Issues](https://github.com/suse/suse-ai-up/issues)
- **Discussions**: [GitHub Discussions](https://github.com/suse/suse-ai-up/discussions)

---

**SUSE AI Universal Proxy** - Making AI model integration secure, scalable, and simple.