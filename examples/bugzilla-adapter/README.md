# Bugzilla Adapter Example

This example demonstrates how to create and use a Bugzilla adapter with the SUSE AI Universal Proxy.

## Overview

The SUSE AI Universal Proxy includes a comprehensive MCP (Model Context Protocol) server registry that allows you to create adapters for various backend services. This example shows how to create an adapter for the SUSE Bugzilla issue tracking system.

## Files

- `create_bugzilla_adapter.sh` - Interactive script for creating a Bugzilla adapter
- `bugzilla_adapter_request.json` - JSON template for adapter creation requests
- `BUGZILLA_ADAPTER_README.md` - Comprehensive documentation and troubleshooting guide

## Prerequisites

1. **SUSE AI Universal Proxy**: Running instance (local or remote)
2. **Bugzilla API Key**: Obtain from https://bugzilla.suse.com/userprefs.cgi?tab=apikey
3. **Bash shell** with curl and jq installed

## Quick Start

### Using the Interactive Script
```bash
cd /path/to/suse-ai-up
./examples/bugzilla-adapter/create_bugzilla_adapter.sh
```

The script will prompt you for:
- SUSE AI Universal Proxy registry URL
- Your user ID
- Adapter name
- Bugzilla server URL

## What the Script Does

1. **URL Configuration**: Prompts for your SUSE AI Universal Proxy registry URL
2. **Registry Check**: Verifies the Bugzilla MCP server is available
3. **Adapter Creation**: Creates a new adapter that automatically deploys as a Docker sidecar
4. **Cleanup**: Removes any existing adapters with the same name
5. **Verification**: Confirms the adapter was created and sidecar deployed
6. **Instructions**: Provides clear next steps for MCP client configuration

## Adapter Configuration

The created adapter will have:
- **Server ID**: `suse-bugzilla`
- **Name**: User-specified (e.g., `my-bugzilla-adapter`)
- **Owner**: User-specified
- **Environment**: Bugzilla server URL
- **Deployment**: Automatic Docker sidecar container using `kskarthik/mcp-bugzilla:latest`

## MCP Client Setup

After running the script, configure your MCP client to connect to:
```
http://your-proxy-host:8911/api/v1/adapters/my-bugzilla-adapter/mcp
```

**Important**: Include this header in all requests:
```
X-User-ID: developer-user
```

## Available Operations

Once connected, the Bugzilla adapter provides:
- Bug search and filtering
- Bug creation and updates
- User and product information
- Comment management
- Status tracking

## Troubleshooting

See `BUGZILLA_ADAPTER_README.md` for detailed troubleshooting information, including:
- API key setup
- Connection issues
- Permission problems
- Debug commands

## Related Examples

- `basic-proxy/` - Basic proxy setup
- `full-stack/` - Complete application stack
- `kubernetes/` - Kubernetes deployment examples

## Learn More

- [SUSE AI Universal Proxy Documentation](../../docs/)
- [Bugzilla MCP Server](https://github.com/openSUSE/mcp-bugzilla)
- [Model Context Protocol](https://modelcontextprotocol.io/)</content>
<parameter name="filePath">examples/bugzilla-adapter/README.md