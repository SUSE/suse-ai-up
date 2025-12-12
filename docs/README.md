# SUSE AI Uniproxy Documentation

Welcome to the SUSE AI Uniproxy documentation. This comprehensive MCP (Model Context Protocol) proxy system enables secure, scalable, and extensible AI model integrations.

## 📖 Documentation Overview

### 🚀 [Getting Started](getting-started.md)
Quick start guide for deploying SUSE AI Uniproxy using Docker, Kubernetes, or Helm.

### 🔌 [API Reference](api/)
Complete API documentation for all endpoints and integration patterns.
- [Overview](api/overview.md) - Core concepts and MCP protocol support
- [Endpoints](api/endpoints.md) - Complete REST API reference
- [Authentication](api/authentication.md) - Auth methods and setup
- [Examples](api/examples.md) - cURL examples and SDK usage

### ⚙️ [Services](services/)
Detailed documentation for each service component.
- [Uniproxy](services/uniproxy.md) - Main MCP proxy service
- [Registry](services/registry.md) - MCP server catalog management
- [Discovery](services/discovery.md) - Network scanning and detection
- [Plugins](services/plugins.md) - Plugin management and routing

### 🏗️ [Integration](integration/)
Deployment and integration guides.
- [Docker](integration/docker.md) - Container deployment
- [Kubernetes](integration/kubernetes.md) - K8s native deployment
- [Helm](integration/helm.md) - Helm chart usage
- [Monitoring](integration/monitoring.md) - Observability setup

### 💡 [Examples](examples/)
Practical examples and use cases.
- [Quick Start](examples/quickstart.md) - 5-minute setup guides
- [Adapters](examples/adapters.md) - Creating MCP server adapters
- [MCP Servers](examples/mcp-servers.md) - Popular MCP server integrations
- [Troubleshooting](examples/troubleshooting.md) - Common issues and solutions

### 🛠️ [Development](development/)
Development and contribution resources.
- [Contributing](development/contributing.md) - Contribution guidelines
- [Architecture](development/architecture.md) - System design details
- [Plugins](development/plugins.md) - Custom plugin development

## 🌟 Key Features

- **🔄 MCP Protocol Support**: Full JSON-RPC 2.0 implementation with HTTP, SSE, and WebSocket transports
- **📚 Server Registry**: Curated catalog of 300+ MCP servers from mcpservers.org
- **🔍 Network Discovery**: Automated MCP server detection and vulnerability assessment
- **🔌 Plugin Ecosystem**: Extensible plugin system for custom functionality
- **🔐 Security First**: OAuth, Bearer tokens, API keys, and TLS everywhere
- **📊 Observability**: Prometheus metrics, structured logging, and health monitoring

## 🏛️ Architecture

SUSE AI Uniproxy uses a unified architecture where all services run together with separate logging:

```
┌─────────────────────────────────────────────────────────────┐
│                      SUSE AI Uniproxy                       │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │
│  │  UNIPROXY   │  │  REGISTRY   │  │ DISCOVERY   │          │
│  │  (Primary)  │  │  (Sidecar)  │  │  (Sidecar)  │          │
│  │             │  │             │  │             │          │
│  │ Port: 8911  │  │ Port: 8913  │  │ Port: 8912  │          │
│  │ HTTPS:3911  │  │ HTTPS:38913 │  │ HTTPS:38912 │          │
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

**Service Startup Order**: Uniproxy → Registry → Discovery → Plugins

## 🚀 Quick Start

### Docker
```bash
docker run -p 8911:8911 suse/suse-ai-up:latest
```

### All Services
```bash
./suse-ai-up all
```

### API Access
- **Main API**: http://localhost:8911
- **Documentation**: http://localhost:8911/docs
- **Health Check**: http://localhost:8911/health

## 📚 Popular MCP Servers

The registry includes 300+ pre-configured MCP servers:

| Category | Examples | Use Cases |
|----------|----------|-----------|
| **Development** | GitHub, GitLab, Linear | Code management, issues, PRs |
| **Productivity** | Notion, Slack, Discord | Document collaboration, communication |
| **Monitoring** | Sentry, DataDog, Grafana | Error tracking, observability |
| **Cloud** | AWS, Azure, GCP | Infrastructure management |
| **AI/ML** | OpenAI, Anthropic, HuggingFace | Model integration |

## 🤝 Contributing

We welcome contributions! See our [Contributing Guide](development/contributing.md) for details.

## 📄 License

Licensed under the Apache License, Version 2.0.