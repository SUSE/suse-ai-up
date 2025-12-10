# Bugzilla Adapter Example

This example demonstrates how to create and use a Bugzilla adapter with the SUSE AI Universal Proxy.

## Overview

The SUSE AI Universal Proxy includes a comprehensive MCP (Model Context Protocol) server registry that allows you to create adapters for various backend services. This example shows how to create an adapter for the SUSE Bugzilla issue tracking system.

## Files

- `bugzilla_adapter_example.sh` - Complete interactive script for creating and testing a Bugzilla adapter
- `create_bugzilla_adapter.sh` - Simple script for quick adapter creation
- `bugzilla_adapter_request.json` - JSON template for adapter creation requests
- `BUGZILLA_ADAPTER_README.md` - Comprehensive documentation and troubleshooting guide

## Prerequisites

1. **SUSE AI Universal Proxy**: Running instance (local or remote)
2. **Bugzilla API Key**: Obtain from https://bugzilla.suse.com/userprefs.cgi?tab=apikey
3. **Bash shell** with curl and jq installed

## Quick Start

### Option 1: Complete Example
```bash
cd /path/to/suse-ai-up
echo "http://your-proxy-host:8911" | bash examples/bugzilla-adapter/bugzilla_adapter_example.sh
```

### Option 2: Simple Creation
```bash
cd /path/to/suse-ai-up
echo "http://your-proxy-host:8911" | bash examples/bugzilla-adapter/create_bugzilla_adapter.sh
```

## What the Scripts Do

1. **URL Configuration**: Prompts for your SUSE AI Universal Proxy URL
2. **Registry Check**: Verifies the Bugzilla MCP server is available
3. **Adapter Creation**: Creates a new adapter with proper configuration
4. **Cleanup**: Removes any existing adapters with the same name
5. **Verification**: Confirms the adapter was created successfully
6. **Instructions**: Provides clear next steps for MCP client configuration

## Adapter Configuration

The created adapter will have:
- **Server ID**: `suse-bugzilla`
- **Name**: `my-bugzilla-adapter`
- **Owner**: `developer-user`
- **Environment**: Bugzilla server URL and API key
- **Deployment**: Local stdio execution

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