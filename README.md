# SUSE AI Universal Proxy

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://golang.org)
[![Docker](https://img.shields.io/badge/Docker-multi--arch-blue.svg)](https://hub.docker.com)

A comprehensive, modular MCP (Model Context Protocol) proxy system that enables secure, scalable, and extensible AI model integrations.

## 🚀 Key Capabilities

**🔄 MCP Proxy Service** - Full-featured HTTP proxy for MCP servers with advanced session management, authentication, and protocol translation.

**🔍 Network Discovery** - Automated network scanning to discover MCP servers, detect authentication types, and assess security vulnerabilities.

**📚 Server Registry** - Multi-source MCP server registry supporting official MCP registry, Docker Hub integration, and custom server management.

**🔌 Plugin Management** - Dynamic plugin system for extending functionality with service registration, health monitoring, and capability routing.

## 🏗️ Architecture

The system uses a **main container + sidecar architecture** where services run as coordinated containers within a single Kubernetes pod:

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

**Service Startup Order**: Proxy → Registry → Discovery → Plugins

## 🏃‍♂️ Quick Start

### Docker (Single Command)
```bash
docker run -p 8080:8080 -p 8911:8911 suse/suse-ai-up:latest
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
go run ./cmd
```

## 📋 Services Overview

### 🔄 MCP Proxy Service
- **Purpose**: HTTP proxy for MCP server communication
- **Features**: Session management, authentication, protocol translation
- **Ports**: HTTP 8080, HTTPS 38080
- **Documentation**: [Proxy Service Guide](docs/services/proxy.md)

### 📚 Registry Service
- **Purpose**: MCP server catalog and management
- **Features**: Multi-source registry, search, validation
- **Ports**: HTTP 8913, HTTPS 38913
- **Documentation**: [Registry Service Guide](docs/services/registry.md)

### 🔍 Discovery Service
- **Purpose**: Network scanning and MCP server detection
- **Features**: CIDR scanning, auth detection, vulnerability assessment
- **Ports**: HTTP 8912, HTTPS 38912
- **Documentation**: [Discovery Service Guide](docs/services/discovery.md)

### 🔌 Plugins Service
- **Purpose**: Dynamic plugin management and routing
- **Features**: Service registration, health monitoring, capability routing
- **Ports**: HTTP 8914, HTTPS 38914
- **Documentation**: [Plugins Service Guide](docs/services/plugins.md)

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