# SUSE AI Universal Proxy - Overview

The SUSE AI Universal Proxy is a reverse proxy and management layer for [Model Context Protocol (MCP)](https://modelcontextprotocol.io/introduction) servers, enabling scalable, session-aware routing and lifecycle management of MCP servers in Kubernetes environments.

## Why a Universal Proxy

The SUSE AI Universal Proxy addresses the growing complexity of deploying and managing AI services in enterprise environments. By providing a unified reverse proxy and management layer for MCP servers, it enables:

- **Scalable Routing**: Session-aware load balancing and routing to MCP server instances
- **Lifecycle Management**: Automated deployment, scaling, and teardown of AI services
- **Registry Management**: Comprehensive MCP server registry with discovery, upload, and search capabilities
- **Enterprise Integration**: Built-in authentication, observability, and security features
- **Multi-Provider Support**: Seamless integration with various AI providers and local models
- **Kubernetes-Native**: Designed for cloud-native deployments with Helm charts and StatefulSets
- **Plugin Architecture**: Extensible microservices framework for pluggable AI capabilities

This solution bridges the gap between AI development and production deployment, making it easier to build and maintain AI-powered applications.

## Plugin Service Framework

The SUSE AI Universal Proxy features a powerful plugin architecture that enables seamless integration of specialized AI services. Services can register with the proxy and automatically receive routed traffic based on API paths.

### Key Features
- **Service Discovery**: Automatic registration and health monitoring of plugin services
- **Dynamic Routing**: Path-based routing to registered services (e.g., `/v1/*` → SmartAgents)
- **Capability Management**: Services declare their API capabilities for intelligent routing
- **Health Monitoring**: Built-in health checks and service status tracking
- **Multi-Service Support**: Support for smartagents, registry, and virtualmcp service types

### Service Types
- **smartagents**: AI orchestration and chat completions
- **registry**: MCP server registry management
- **virtualmcp**: VM-based MCP server management (future)

## Key Concepts

- **MCP Server**: A server implementing the Model Context Protocol, which typically exposes SSE or streamable HTTP endpoints.
- **Adapters**: Logical resources representing MCP servers in the gateway, managed under the `/adapters` scope. Designed to coexist with other resource types (e.g., `/agents`) in a unified AI development platform.
- **Session-Aware Stateful Routing**: Ensures that all requests with a given `session_id` are consistently routed to the same MCP server instance.

## Architecture Overview

### System Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    SUSE AI Universal Proxy                       │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │                    Plugin Service Framework                 │ │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────┐  │ │
│  │  │   SmartAgents   │  │    Registry     │  │ VirtualMCP  │  │ │
│  │  │   Service       │  │   Service       │  │  Service    │  │ │
│  │  └─────────────────┘  └─────────────────┘  └─────────────┘  │ │
│  │           │                       │                       │  │ │
│  └───────────┼───────────────────────┼───────────────────────┘  │ │
│              │                       │                          │ │
│  ┌───────────▼───────────────────────▼─────────────────────────┐ │ │
│  │                Dynamic Router & Load Balancer               │ │
│  └─────────────────────────────────────────────────────────────┘ │ │
│  ┌─────────────────────────────────────────────────────────────┐ │ │
│  │              Service Discovery & Health Monitor             │ │ │
│  └─────────────────────────────────────────────────────────────┘ │ │
└─────────────────────────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                    External Clients                             │ │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │ │
│  │   VS Code       │  │   Web Apps      │  │   CLI Tools     │  │ │
│  │   MCP Clients   │  │   REST APIs     │  │   curl/wget     │  │ │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘  │ │
└─────────────────────────────────────────────────────────────────┘
```

### Plugin Framework Components

```
Plugin Service Framework
├── Service Manager
│   ├── Service Registry (in-memory store)
│   ├── Health Monitor (30s intervals)
│   └── Capability Matcher
├── Dynamic Router
│   ├── Path-based Routing (/v1/* → SmartAgents)
│   ├── Load Balancing (round-robin)
│   └── Request Forwarding
├── Plugin Services
│   ├── SmartAgents (AI chat, model management)
│   ├── Registry (MCP server catalog)
│   └── VirtualMCP (VM-based MCP servers)
└── Service Discovery
    ├── Auto-registration (PROXY_URL env var)
    ├── Manual registration (/api/v1/plugins/register)
    └── Service health checks (/health)
```

### Data Flow: Service Registration & Routing

```
1. Service Registration Flow:
   SmartAgents Service ──PROXY_URL=http://localhost:8911──► Proxy
        │
        ▼
   ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
   │  Register with  │────►│   Validate &    │────►│   Store in      │
   │   Proxy via     │     │   Health Check  │     │   Registry      │
   │   HTTP POST     │     │                 │     │                 │
   └─────────────────┘     └─────────────────┘     └─────────────────┘
        ▲                       ▲                       ▲
        │                       │                       │
   Capabilities: /v1/*     Health: /health        Service ID: smartagents-*

2. Request Routing Flow:
   Client Request ──/v1/models──► Proxy Router
        │
        ▼
   ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
   │   Match Path    │────►│   Select        │────►│   Forward to    │
   │   Pattern       │     │   Service       │     │   SmartAgents   │
   │   /v1/models    │     │   Instance      │     │   Service       │
   └─────────────────┘     └─────────────────┘     └─────────────────┘
        ▲                       ▲                       ▲
        │                       │                       │
   Route to: smartagents-*   Load Balance         Response: model list
```

### Service Discovery & Health Monitoring

```
Service Discovery Process:
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Network   │────►│   Scan for  │────►│   Detect    │
│   Scan      │     │   MCP        │     │   Services  │
│   (CIDR)    │     │   Servers    │     │   on Port   │
└─────────────┘     └─────────────┘     └─────────────┘
      │                       │                       │
      ▼                       ▼                       ▼
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Register  │◄────│   Create    │◄────│   Validate  │
│   with      │     │   Adapter   │     │   Endpoints │
│   Proxy     │     │   Config    │     │   /mcp      │
└─────────────┘     └─────────────┘     └─────────────┘

Health Monitoring Loop:
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Timer     │────►│   Health    │────►│   Update    │
│   (30s)     │     │   Check     │     │   Status    │
└─────────────┘     └─────────────┘     └─────────────────┐
      ▲                       ▲                           │
      │                       │                    ┌─────────────┐
      └───────────────────────┼────────────────────│   Mark      │
                              │                    │   Unhealthy │
                              └────────────────────│   if Failed │
                                                   └─────────────┘
```

### Component Interactions

```
Plugin Service Lifecycle:
1. Registration ──► Health Check ──► Active Routing ──► Deregistration
      │                    │                │                │
      ▼                    ▼                ▼                ▼
   HTTP POST          30s intervals    Path matching    HTTP DELETE
   /register           /health          /v1/* routes     /services/{id}

Error Handling Flow:
Client Request ──► Router ──► Service ──► Success Response
      │                │          │
      ▼                ▼          ▼
   404 Not Found   503 Unhealthy  500 Internal Error
   (no route)      (health fail)  (service error)

Load Balancing:
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Round Robin   │────►│   Least         │────►│   IP Hash       │
│   Distribution  │     │   Connections   │     │   Affinity      │
└─────────────────┘     └─────────────────┘     └─────────────────┘
      ▲                       ▲                       ▲
      │                       │                       │
   Default Strategy    For stateful APIs     For session stickiness
```

## Features

### Control Plane – RESTful APIs for MCP Server Management

- `POST /adapters` — Deploy and register a new MCP server.
- `GET /adapters` — List all MCP servers the user can access.
- `GET /adapters/{name}` — Retrieve metadata for a specific adapter.
- `GET /adapters/{name}/status` — Check the deployment status.
- `GET /adapters/{name}/logs` — Access the server's running logs.
- `PUT /adapters/{name}` — Update the deployment.
- `DELETE /adapters/{name}` — Remove the server.

### Data Plane – Gateway Routing for MCP Servers

- `GET /adapters/{name}/sse` — Establish an initial SSE connection.
- `POST /adapters/{name}/messages` — Send subsequent requests using `session_id`.
- `POST /adapters/{name}/mcp` — Establish a streamable HTTP connection.

### Session Management – MCP Session Lifecycle

- `GET /adapters/{name}/sessions` — List all active sessions for an adapter.
- `GET /adapters/{name}/sessions/{sessionId}` — Get detailed information about a specific session.
- `POST /adapters/{name}/sessions` — Reinitialize/create a new session for an adapter.
- `DELETE /adapters/{name}/sessions/{sessionId}` — Invalidate and remove a specific session.
- `DELETE /adapters/{name}/sessions` — Remove all sessions for an adapter.

### Discovery – Network Scanning for MCP Servers

- `POST /scan` — Start a network scan to discover MCP servers on specified IP ranges and ports.
- `GET /scan/{scanId}` — Get the status and results of a specific scan.
- `GET /servers` — List all discovered MCP servers.
- `POST /register` — Register a discovered server as an adapter.

### Additional Capabilities

- Authentication and authorization support (production mode).
- Stateless reverse proxy with a distributed session store (production mode).
- Kubernetes-native deployment using StatefulSets and headless services.