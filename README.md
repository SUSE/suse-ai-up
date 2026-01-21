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

## 📖 Documentation

- **[QUICKSTART](QUICKSTART.md)** - Get started quickly with SUSE AI Universal Proxy
- **[REGISTRY](REGISTRY.md)** - Learn about MCP server registry management
- **[SECURITY](SECURITY.md)** - Security guidelines and best practices
- **[AUTHENTICATION](AUTHENTICATION.md)** - Authentication and authorization options

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

## Setup SUSE AI Universal Proxy (Helm installation)

1. Open a local terminal 
2. Clone the repository: `https://github.com/suse/suse-ai-up` (branch: main)
3. enter in the folder `suse-ai-up`
4. In values.yaml, set:
   - `service.type: LoadBalancer`
   - `auth.method: development` (for no auth)

Install using the helm chart:

```bash
helm install suse-ai-up ./charts/suse-ai-up
```
5. Wait for the installation to be completed

Get Service IP
```bash
kubectl get svc suse-ai-up-service -n suse-ai-up -o jsonpath='{.status.loadBalancer.ingress[0].ip}'
```

## (Alternative) Setup SUSE AI Universal Proxy (in Rancher)

1. In Rancher, add repository: `https://github.com/suse/suse-ai-up` (branch: main)
2. Go to Apps → Charts
3. Find and install "SUSE AI Universal Proxy"
4. Click Install and wait for completion


## Setup SUSE AI Universal Proxy UI

1. In Rancher, add repository: `https://github.com/suse/suse-ai-up-ext`(branch: v0.1.0)
2. Go to Extensions
3. Find and install "SUSE AI Universal Proxy"

## Verify Installation and Access Swagger Docs

Access API documentation:
```
http://{IP ADDRESS}:8911/docs/index.html
```

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

- **Issues**: [GitHub Issues](https://github.com/suse/suse-ai-up/issues)
- **Discussions**: [GitHub Discussions](https://github.com/suse/suse-ai-up/discussions)

---

**SUSE AI Universal Proxy** - Making AI model integration secure, scalable, and simple.
