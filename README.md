# SUSE AI Universal Proxy

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://golang.org)
[![Docker](https://img.shields.io/badge/Docker-multi--arch-blue.svg)](https://hub.docker.com)

A comprehensive, modular MCP (Model Context Protocol) proxy system that enables secure, scalable, and extensible AI model integrations.

## 🚀 Key Capabilities

**🔄 MCP Proxy Service** - Full-featured HTTP proxy for MCP servers with advanced session management, authentication, and protocol translation.

**🔍 Network Discovery** - Automated network scanning to discover MCP servers, detect authentication types, and assess security vulnerabilities.

**📚 Server Registry** - Curated registry of MCP Servers, including GitHub, SUSE MCP's, Atlassian, Gitea, and 20+ other popular services (yes you may contribute to the list!).

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

### Kubernetes + Helm
```bash
helm install suse-ai-up charts/suse-ai-up
```
If you use Rancher is even more simpler:
1. Add https://github.com/SUSE/suse-ai-up to the respositories in your selected cluster (local or downstream)
2. Use "main" as Git branch
3. Create and wait for the repository to show the status "Active"
4. Click on "Charts" in the cluster Apps
5. Follow the wizard to install.

### Local Development
```bash
git clone https://github.com/suse/suse-ai-up.git
cd suse-ai-up
go run ./cmd/uniproxy
```
Universal Proxy require Kubernets so the ideal development way is to deploy the helm chart in kubernetes

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

### 🚀 Quick Examples

Check the file EXAMPLES.md

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
