# SUSE AI Universal Proxy Documentation

Welcome to the comprehensive documentation for SUSE AI Universal Proxy.

## 📚 Core Documentation

### Getting Started
- **[Getting Started](getting-started.md)** - Installation, setup, and quick start guide
- **[Overview](overview.md)** - Architecture, concepts, and design principles

### API Reference
- **[API Reference](api-reference.md)** - Complete REST API documentation
- **[Swagger Documentation](swagger.json)** - OpenAPI/Swagger specification

### Features & Services
- **[Adapters Guide](adapters.md)** - Adapter configuration and management
- **[Discovery Service](discovery.md)** - Network discovery and auto-registration
- **[Registry Management](registry.md)** - MCP server registry operations
- **[Examples](examples.md)** - Usage examples and tutorials

### Security & Authentication
- **[Security](security.md)** - Security features and best practices
- **[OAuth Implementation](oauth-implementation.md)** - OAuth 2.1 compliant token management system

## 🔧 Development

### Development Resources
- **[Contributing Guide](../CONTRIBUTING.md)** - Development guidelines and contribution process
- **[License](../LICENSE.md)** - Apache 2.0 license information

### API Documentation
- **Interactive API Docs** - Start the service and visit `http://localhost:8911/docs`
- **OpenAPI Specification** - [swagger.json](swagger.json)

## 🚀 Quick Start

1. **Start the service**:
   ```bash
   go run cmd/service/main.go
   ```

2. **Access API documentation**:
   - Interactive docs: http://localhost:8911/docs
   - API endpoints: http://localhost:8911/

3. **Common operations**:
   - List adapters: `GET /adapters`
   - Discover servers: `POST /scan`
   - Browse registry: `GET /registry/browse`

## 📖 Featured Topics

### OAuth 2.1 Token Management
New OAuth 2.1 compliant token management system that eliminates "INVALID_TOKEN" errors:

- **JWT-based authentication** with RSA-256 signing
- **Token distribution APIs** for client integration
- **Automatic security** for high-risk discovered servers
- **Backward compatibility** with existing legacy tokens

📖 **[Read OAuth Implementation Guide](oauth-implementation.md)**

### Adapter Authentication
Configure adapters with different authentication methods:

- **Bearer tokens** (legacy and JWT formats)
- **OAuth integration** with external providers
- **Automatic token generation** for discovered servers

📖 **[Read Adapters Guide](adapters.md)**

### Network Discovery
Automatically discover and register MCP servers on your network:

- **CIDR scanning** for comprehensive network discovery
- **Vulnerability assessment** with automatic security enhancement
- **Auto-registration** with token-based authentication

📖 **[Read Discovery Guide](discovery.md)**

---

## 🔗 Related Resources

- **[Main Project README](../README.md)** - Project overview and features
- **[GitHub Repository](https://github.com/suse/suse-ai-up)** - Source code and issues
- **[GitHub Discussions](https://github.com/suse/suse-ai-up/discussions)** - Community discussions

---

*Last updated: November 2025*