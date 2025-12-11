# SUSE AI Universal Proxy - Examples

This document provides examples of how to test various MCP (Model Context Protocol) servers using the SUSE AI Universal Proxy.

## Prerequisites

1. **Running SUSE AI Universal Proxy**: Make sure the proxy service is running and accessible
2. **mcpinspector**: Install mcpinspector for testing MCP connections:
   ```bash
   npm install -g @modelcontextprotocol/inspector
   ```

## Example MCP Servers

### 1. SUSE Bugzilla MCP

**Description**: Official SUSE MCP server for Bugzilla issue tracking and bug management.

**Search for the MCP in Registry**:
```bash
curl -X GET "http://localhost:8913/api/v1/registry/search?q=bugzilla" \
  -H "Content-Type: application/json"
```

**Create Adapter**:
```bash
curl -X POST "http://localhost:8911/api/v1/adapters" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "bugzilla-adapter",
    "serverId": "suse-bugzilla",
    "environmentVariables": {
      "BUGZILLA_URL": "https://bugzilla.suse.com"
    }
  }'
```

**Connect using mcpinspector**:
```bash
mcpinspector "http://localhost:8911/api/v1/adapters/bugzilla-adapter/connect"
```

### 2. SUSE Uyuni MCP

**Description**: Official SUSE MCP server for Uyuni server management, patch deployment, and system administration.

**Prepare Configuration File**:
Create a `uyuni-config.env` file with your Uyuni server credentials:

```bash
# Required: Basic server parameters
UYUNI_SERVER=your-uyuni-server.example.com:443
UYUNI_USER=admin
UYUNI_PASS=your-admin-password

# Optional: Set to 'false' to disable SSL certificate verification. Defaults to 'true'.
UYUNI_MCP_SSL_VERIFY=false

# Optional: Set to 'true' to enable tools that perform write actions. Defaults to 'false'.
UYUNI_MCP_WRITE_TOOLS_ENABLED=false

# Optional: Set the transport protocol. Can be 'stdio' (default) or 'http'.
UYUNI_MCP_TRANSPORT=http

# Optional: Set the path for the server log file. Defaults to logging to the console.
UYUNI_MCP_LOG_FILE_PATH=/var/log/mcp-server-uyuni.log

# Required to bootstrap new systems into Uyuni via the 'add_system' tool.
UYUNI_SSH_PRIV_KEY="-----BEGIN OPENSSH PRIVATE KEY-----
your-private-key-here
-----END OPENSSH PRIVATE KEY-----"
UYUNI_SSH_PRIV_KEY_PASS="your-key-passphrase"
```

**Search for the MCP in Registry**:
```bash
curl -X GET "http://localhost:8913/api/v1/registry/search?q=uyuni" \
  -H "Content-Type: application/json"
```

**Create Adapter**:
```bash
curl -X POST "http://localhost:8911/api/v1/adapters" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "uyuni-adapter",
    "serverId": "suse-uyuni",
    "environmentVariables": {
      "UYUNI_SERVER": "your-uyuni-server.example.com:443",
      "UYUNI_USER": "admin",
      "UYUNI_PASS": "your-admin-password",
      "UYUNI_MCP_SSL_VERIFY": "false",
      "UYUNI_MCP_TRANSPORT": "http"
    }
  }'
```

**Connect using mcpinspector**:
```bash
mcpinspector "http://localhost:8911/api/v1/adapters/uyuni-adapter/connect"
```

### 3. Sequential Thinking MCP

**Description**: An MCP server implementation that provides a tool for dynamic and reflective problem-solving through a structured thinking process.

**Search for the MCP in Registry**:
```bash
curl -X GET "http://localhost:8913/api/v1/registry/search?q=sequential-thinking" \
  -H "Content-Type: application/json"
```

**Create Adapter** (Local Stdio - requires installation)**:
```bash
# First, install the MCP server locally
npm install -g mcp-sequential-thinking

# Then create the adapter
curl -X POST "http://localhost:8911/api/v1/adapters" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "sequential-thinking-adapter",
    "serverId": "sequential-thinking"
  }'
```

**Connect using mcpinspector**:
```bash
mcpinspector "http://localhost:8911/api/v1/adapters/sequential-thinking-adapter/connect"
```

## Common Operations

### List All Adapters
```bash
curl -X GET "http://localhost:8911/api/v1/adapters" \
  -H "Content-Type: application/json"
```

### Get Adapter Details
```bash
curl -X GET "http://localhost:8911/api/v1/adapters/{adapter-name}" \
  -H "Content-Type: application/json"
```

### Delete Adapter
```bash
curl -X DELETE "http://localhost:8911/api/v1/adapters/{adapter-name}" \
  -H "Content-Type: application/json"
```

### Search Registry
```bash
curl -X GET "http://localhost:8913/api/v1/registry/search?q={search-term}" \
  -H "Content-Type: application/json"
```

## Troubleshooting

### Adapter Creation Fails
- Check that the MCP server exists in the registry
- Verify environment variables are correctly set
- Ensure the proxy service has proper RBAC permissions for sidecar deployments

### Connection Issues
- Verify the adapter is in "running" state
- Check proxy service logs for errors
- Ensure mcpinspector is properly installed

### Sidecar Deployment Issues
- Check Kubernetes RBAC permissions
- Verify the `suse-ai-up-mcp` namespace exists
- Ensure Docker images are accessible

## Advanced Usage

### Custom Environment Variables
```bash
curl -X POST "http://localhost:8911/api/v1/adapters" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "custom-adapter",
    "serverId": "server-id",
    "environmentVariables": {
      "CUSTOM_VAR": "value",
      "ANOTHER_VAR": "another-value"
    }
  }'
```

### Using Different MCP Servers
Replace `serverId` with any MCP server ID from the registry search results.