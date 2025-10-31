# SUSE AI Universal Proxy Examples

This directory contains example implementations and usage patterns for the SUSE AI Universal Proxy ecosystem.

## Structure

- `proxy/` - Examples for the SUSE AI Universal Proxy Control Plane
- `smartagents/` - Examples for the SUSE AI Universal Proxy Smart Agents
- `virtualmcp/` - Examples for Virtual MCP implementations (coming soon)

## Proxy Examples

The proxy examples demonstrate how to interact with the Control Plane API for managing MCP server deployments and proxying requests.

### Python Client Example

Located in `proxy/src/main.py`, this example shows how to:
- Connect to the proxy service
- List available MCP adapters
- Create and manage MCP server deployments

## SmartAgents Examples

The smartagents examples demonstrate how to build and deploy smart agents that integrate with the proxy ecosystem.

### Chat Client Example

Located in `smartagents/`, this example shows how to:
- Create a chat client that mimics OpenAI API compatibility
- Integrate with the proxy for MCP server access
- Handle authentication and session management

## Getting Started

1. Choose the appropriate example directory based on your use case
2. Follow the README.md in each subdirectory for setup instructions
3. Ensure the corresponding service is running (proxy on port 8911, smartagents on port 8910)

## Contributing

When adding new examples:
1. Create a new subdirectory under the appropriate service directory
2. Include a comprehensive README.md with setup and usage instructions
3. Ensure examples are well-documented and follow best practices
4. Test examples against the latest service versions