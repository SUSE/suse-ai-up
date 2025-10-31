# SUSE AI Universal Proxy - Documentation

Welcome to the comprehensive documentation for the SUSE AI Universal Proxy project. This documentation is organized by service and function to help you find the information you need quickly.

## 📚 Documentation Structure

### 🏠 Project Overview
- **[Main README](../README.md)** - Project overview, quick start, and service descriptions
- **[Contributing](../CONTRIBUTING.md)** - How to contribute to the project
- **[License](../LICENSE.md)** - Apache 2.0 license information

### 🔀 Proxy Service (`docs/proxy/`)

The core reverse proxy and management layer for MCP servers.

#### 📖 Overview & Architecture
- **[Overview](proxy/overview.md)** - Complete proxy service overview, architecture, and key concepts
- **[Getting Started](proxy/getting-started.md)** - Installation, setup, and basic usage
- **[API Reference](proxy/api-reference.md)** - Complete API documentation with examples
- **[Examples](proxy/examples.md)** - Practical usage examples and tutorials
- **[Security](proxy/security.md)** - Security features, OAuth flows, and best practices

### 🤖 Smart Agents Service (`docs/smartagents/`)

AI orchestration and chat completions with supervisor-worker architecture.

#### 📖 Service Documentation
- **[Overview](smartagents/overview.md)** - Service features, architecture, and capabilities
- **[Getting Started](smartagents/getting-started.md)** - Installation, configuration, and first agent setup
- **[Examples](smartagents/examples.md)** - Agent creation, usage patterns, and integrations

### 🔧 Virtual MCP Service (`docs/virtualmcp/`)

Code-free MCP server generation from APIs and databases.

#### 📖 Service Documentation
- **[Overview](virtualmcp/overview.md)** - Service architecture, features, and use cases
- **[Getting Started](virtualmcp/getting-started.md)** - Installation, configuration, and first server generation
- **[Examples](virtualmcp/examples.md)** - API conversion, database integration, and advanced configurations

### 📚 MCP Registry (`docs/registry.md`)

Comprehensive MCP server registry documentation including:
- Registry integration and API endpoints
- Bulk upload and search capabilities
- Migration guides and configuration
- Server discovery and management

## 🚀 Quick Start Guides

### New to the Project?
1. **Read the [Main README](../README.md)** for project overview
2. **Choose your service:**
   - **Proxy Service**: Start with [Proxy Getting Started](proxy/getting-started.md)
   - **Smart Agents**: Start with [Smart Agents Getting Started](smartagents/getting-started.md)
   - **Virtual MCP**: Start with [Virtual MCP Getting Started](virtualmcp/getting-started.md)
3. **Follow the examples** in the respective service's examples documentation

### Setting Up Development Environment
1. **Prerequisites**: Check [Proxy Getting Started](proxy/getting-started.md#prerequisites) or [Smart Agents Getting Started](smartagents/getting-started.md#prerequisites)
2. **Local Setup**: Follow the installation guides for your chosen service
3. **First Test**: Use the provided examples to verify your setup

### API Integration
- **Proxy APIs**: See [API Reference](proxy/api-reference.md) for complete endpoint documentation
- **Smart Agents APIs**: Compatible with OpenAI API - see [Examples](smartagents/examples.md)
- **Authentication**: Review [Security](proxy/security.md) for OAuth and security features

## 🔍 Finding Information

### By Task
- **Installation & Setup**: Service-specific getting started guides
- **API Usage**: API reference and examples sections
- **Configuration**: Getting started guides and overview sections
- **Security**: Security documentation for each service
- **Troubleshooting**: Examples sections include troubleshooting tips

### By Service Component
- **MCP Server Management**: [Proxy Overview](proxy/overview.md) and [API Reference](proxy/api-reference.md)
- **AI Agent Orchestration**: [Smart Agents Overview](smartagents/overview.md)
- **Virtual MCP Generation**: [Virtual MCP Overview](virtualmcp/overview.md) and [Examples](virtualmcp/examples.md)
- **Registry Management**: [Registry Documentation](docs/registry.md)
- **Plugin Architecture**: [Proxy Overview](proxy/overview.md#plugin-service-framework)
- **Security Features**: [Proxy Security](proxy/security.md)

### By User Type
- **Developers**: Start with getting started guides, then API references
- **System Administrators**: Focus on security, deployment, and configuration
- **DevOps Engineers**: Review deployment examples and monitoring sections
- **Contributors**: Check [Contributing](../CONTRIBUTING.md) guidelines

## 📋 Common Workflows

### 1. Local Development Setup
```
1. Choose service (Proxy or Smart Agents)
2. Follow prerequisites in getting started guide
3. Install and configure the service
4. Create first configuration (agent/adapter)
5. Test with provided examples
6. Review API documentation for integration
```

### 2. API Integration
```
1. Review API reference for target service
2. Check examples for usage patterns
3. Implement authentication if required
4. Test integration with examples
5. Handle errors and edge cases
```

### 3. Production Deployment
```
1. Review security documentation
2. Configure environment variables
3. Set up monitoring and logging
4. Test deployment with examples
5. Configure backup and recovery
```

## 🔗 Cross-References

### Related Documentation
- **MCP Specification**: [Model Context Protocol](https://modelcontextprotocol.io/)
- **OpenAI API**: Compatible endpoints documented in [Smart Agents Examples](smartagents/examples.md)
- **OAuth 2.1**: Security implementation details in [Proxy Security](proxy/security.md)

### Service Interactions
- **Proxy ↔ Smart Agents**: Plugin registration described in [Proxy Overview](proxy/overview.md#plugin-service-framework)
- **Registry ↔ Proxy**: Integration documented in [Registry](docs/registry.md#architecture-overview)
- **Registry ↔ Smart Agents**: API usage in [Smart Agents Examples](smartagents/examples.md#mcp-registry-examples)

## 🆘 Getting Help

### Documentation Issues
- **Missing Information**: Check if the topic is covered in a different section
- **Outdated Examples**: Verify with the latest getting started guides
- **API Changes**: Review API reference for current specifications

### Community Support
- **GitHub Issues**: Report bugs or request documentation improvements
- **GitHub Discussions**: Ask questions and share experiences
- **Contributing**: See [Contributing](../CONTRIBUTING.md) for how to help improve docs

## 📝 Documentation Conventions

### Code Examples
- **Shell commands**: Use backticks for inline commands, code blocks for multi-line
- **API requests**: Include complete curl examples with headers and expected responses
- **Configuration**: Show both JSON and environment variable formats
- **Error handling**: Include common error responses and troubleshooting steps

### Navigation
- **Breadcrumbs**: Show location in documentation hierarchy
- **Cross-references**: Link to related sections and external resources
- **Table of contents**: Auto-generated for easy navigation
- **Search-friendly**: Use descriptive headings and consistent terminology

### Updates
- **Version notes**: Highlight changes in new versions
- **Deprecation warnings**: Clearly mark deprecated features
- **Migration guides**: Provide step-by-step upgrade instructions

---

## 🎯 Next Steps

Now that you understand the documentation structure:

1. **Choose your starting point** based on your needs
2. **Follow the getting started guide** for your chosen service
3. **Explore the examples** to understand capabilities
4. **Dive into API references** for integration details
5. **Review security documentation** for production deployment

Happy exploring! 🚀