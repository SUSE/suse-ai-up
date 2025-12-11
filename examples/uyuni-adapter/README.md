# SUSE Uyuni MCP Adapter Example

This example demonstrates how to create and use a SUSE Uyuni MCP adapter with the SUSE AI Universal Proxy.

## Overview

SUSE Uyuni is a systems management solution for managing Linux systems. The Uyuni MCP server provides tools to interact with Uyuni servers for system management, patch management, and configuration management.

## Prerequisites

- Running SUSE AI Universal Proxy instance
- Access to a Uyuni server
- Uyuni server credentials (username, password, server URL)

## Features

The Uyuni MCP adapter provides the following tools:

- **get_list_of_active_systems**: Retrieve list of active systems
- **get_cpu_of_a_system**: Get CPU information for a specific system
- **get_all_systems_cpu_info**: Get CPU information for all systems
- **check_system_updates**: Check for available updates on a system
- **check_all_systems_for_updates**: Check updates for all systems
- **schedule_apply_pending_updates_to_system**: Schedule update application
- **schedule_apply_specific_update**: Apply specific updates
- **add_system**: Add a new system to Uyuni
- **remove_system**: Remove a system from Uyuni
- **get_systems_needing_security_update_for_cve**: Find systems needing security updates
- **get_systems_needing_reboot**: Find systems requiring reboot
- **schedule_system_reboot**: Schedule system reboots
- **cancel_action**: Cancel scheduled actions
- **list_all_scheduled_actions**: List all scheduled actions
- **list_activation_keys**: List available activation keys

## Creating the Adapter

### 1. Prepare Configuration

Create a JSON request file with your Uyuni server details:

```json
{
  "mcpServerId": "suse-uyuni",
  "name": "my-uyuni-adapter",
  "description": "Uyuni adapter for system management",
  "environmentVariables": {
    "UYUNI_SERVER": "https://uyuni.example.com",
    "UYUNI_USER": "admin",
    "UYUNI_PASS": "your-password",
    "UYUNI_SSH_PRIV_KEY": "-----BEGIN OPENSSH PRIVATE KEY-----\n...",
    "UYUNI_SSH_PRIV_KEY_PASS": "",
    "UYUNI_MCP_SSL_VERIFY": "true",
    "UYUNI_MCP_WRITE_TOOLS_ENABLED": "false"
  }
}
```

**Required Environment Variables:**
- `UYUNI_SERVER`: URL of your Uyuni server
- `UYUNI_USER`: Uyuni username
- `UYUNI_PASS`: Uyuni password

**Optional Environment Variables:**
- `UYUNI_SSH_PRIV_KEY`: SSH private key for system registration (base64 encoded)
- `UYUNI_SSH_PRIV_KEY_PASS`: SSH key passphrase
- `UYUNI_MCP_SSL_VERIFY`: Enable/disable SSL verification (default: true)
- `UYUNI_MCP_WRITE_TOOLS_ENABLED`: Enable write operations (default: false)

### 2. Create the Adapter

Send a POST request to create the adapter:

```bash
curl -X POST http://localhost:8913/api/v1/adapters \
  -H "Content-Type: application/json" \
  -d @uyuni_adapter_request.json
```

**Response:**
```json
{
  "id": "my-uyuni-adapter",
  "mcpServerId": "suse-uyuni",
  "mcpClientConfig": {
    "mcpServers": [
      {
        "auth": {
          "token": "adapter-session-token",
          "type": "bearer"
        },
        "url": "http://localhost:8911/api/v1/adapters/my-uyuni-adapter/mcp"
      }
    ]
  },
  "capabilities": {
    "serverInfo": {
      "name": "my-uyuni-adapter",
      "version": "1.0.0",
      "protocol": "",
      "capabilities": null
    },
    "tools": [
      {
        "name": "example_tool",
        "description": "Example tool from remote server",
        "input_schema": {
          "properties": {
            "input": {
              "type": "string"
            }
          },
          "type": "object"
        }
      }
    ],
    "lastRefreshed": "2025-12-10T19:38:28.396835+01:00"
  },
  "status": "ready",
  "createdAt": "2025-12-10T19:38:28.396836+01:00"
}
```

## Using the Adapter

### MCP Client Configuration

Use the `mcpClientConfig` from the response to configure your MCP client:

```json
{
  "mcpServers": {
    "uyuni": {
      "url": "http://localhost:8911/api/v1/adapters/my-uyuni-adapter/mcp",
      "headers": {
        "Authorization": "Bearer adapter-session-token"
      }
    }
  }
}
```

### Example MCP Requests

#### Initialize the connection:
```json
{
  "jsonrpc": "2.0",
  "method": "initialize",
  "params": {
    "protocolVersion": "2024-11-05",
    "capabilities": {},
    "clientInfo": {
      "name": "example-client",
      "version": "1.0.0"
    }
  },
  "id": 1
}
```

#### List available tools:
```json
{
  "jsonrpc": "2.0",
  "method": "tools/list",
  "id": 2
}
```

#### Call a tool (get active systems):
```json
{
  "jsonrpc": "2.0",
  "method": "tools/call",
  "params": {
    "name": "get_list_of_active_systems",
    "arguments": {}
  },
  "id": 3
}
```

## Testing with MCP Inspector

You can test the adapter using the MCP Inspector:

```bash
npx @modelcontextprotocol/inspector \
  --proxy http://localhost:8911/api/v1/adapters/my-uyuni-adapter/mcp \
  --header "Authorization: Bearer adapter-session-token"
```

## Security Considerations

- **SSL Verification**: Enable `UYUNI_MCP_SSL_VERIFY=true` for production
- **Write Operations**: Only enable `UYUNI_MCP_WRITE_TOOLS_ENABLED=true` when necessary
- **SSH Keys**: Store SSH private keys securely, never in version control
- **Credentials**: Use strong passwords and consider using environment variables

## Troubleshooting

### Connection Issues
- Verify Uyuni server URL and credentials
- Check network connectivity to Uyuni server
- Ensure SSL certificates are valid (or disable verification for testing)

### Authentication Errors
- Confirm username and password are correct
- Check if 2FA is enabled on the Uyuni server
- Verify API access permissions

### Tool Execution Errors
- Some tools require write permissions - enable `UYUNI_MCP_WRITE_TOOLS_ENABLED`
- SSH operations require valid SSH keys and network access
- System registration requires proper activation keys

## Advanced Configuration

### SSH Key Setup

For system registration functionality, you need to provide an SSH private key:

1. Generate or use existing SSH key:
```bash
ssh-keygen -t rsa -b 4096 -f uyuni_key
```

2. Convert to single-line format:
```bash
awk 'NF {printf "%s\\n", $0}' uyuni_key
```

3. Add to environment variables:
```json
{
  "UYUNI_SSH_PRIV_KEY": "-----BEGIN OPENSSH PRIVATE KEY-----\n...",
  "UYUNI_SSH_PRIV_KEY_PASS": ""
}
```

### Write Operations

To enable destructive operations (system removal, updates, reboots):

```json
{
  "UYUNI_MCP_WRITE_TOOLS_ENABLED": "true"
}
```

⚠️ **Warning**: Enabling write operations allows the MCP server to perform destructive actions on your Uyuni-managed systems.

## Integration Examples

### With Open WebUI

Add the adapter to your Open WebUI MCP configuration:

```json
{
  "mcpServers": {
    "uyuni": {
      "url": "http://localhost:8911/api/v1/adapters/my-uyuni-adapter/mcp",
      "headers": {
        "Authorization": "Bearer adapter-session-token"
      }
    }
  }
}
```

### With Claude Desktop

Add to your Claude Desktop configuration:

```json
{
  "mcpServers": {
    "uyuni": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/inspector", "--proxy", "http://localhost:8911/api/v1/adapters/my-uyuni-adapter/mcp", "--header", "Authorization: Bearer adapter-session-token"]
    }
  }
}
```

## Support

For issues with the Uyuni MCP server itself, please refer to:
- [Uyuni Project](https://www.uyuni-project.org/)
- [Uyuni Documentation](https://www.uyuni-project.org/uyuni-docs/)
- [Uyuni MCP Server Repository](https://github.com/uyuni-project/mcp-server-uyuni)

For issues with the SUSE AI Universal Proxy, please check the main project documentation.</content>
<parameter name="filePath">examples/uyuni-adapter/README.md