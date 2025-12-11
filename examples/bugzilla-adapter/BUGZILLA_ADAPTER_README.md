# Bugzilla Adapter Creation Example

This guide demonstrates how to create and use a Bugzilla adapter with the SUSE AI Universal Proxy.

## Prerequisites

1. **Running SUSE AI Universal Proxy**:
   ```bash
   docker run -p 8911-8914:8911-8914 ghcr.io/alessandro-festa/suse-ai-up:latest
   ```

2. **Bugzilla API Key**: Obtain an API key from your Bugzilla instance
   - Visit: https://bugzilla.suse.com/userprefs.cgi?tab=apikey
   - Generate a new API key

## Quick Start

### Using the Automated Script

```bash
# Run the complete example (will prompt for proxy URL)
./bugzilla_adapter_example.sh

# Or run the simple version
./create_bugzilla_adapter.sh
```

**Note**: Both scripts will prompt you to enter the SUSE AI Universal Proxy base URL (e.g., `http://localhost:8911` or `https://my-proxy.example.com:8911`).

### Manual Creation

```bash
# 1. Check Bugzilla server availability (replace localhost:8913 with your registry URL)
curl http://your-registry-host:8913/api/v1/registry/browse?q=bugzilla

# 2. Create the adapter (replace localhost:8913 with your registry URL)
curl -X POST http://your-registry-host:8913/api/v1/adapters \
  -H "Content-Type: application/json" \
  -H "X-User-ID: your-user-id" \
  -d @bugzilla_adapter_request.json
```

**Note**: Replace `your-registry-host:8913` with your actual SUSE AI Universal Proxy registry URL.

## Adapter Configuration

### Required Parameters

| Parameter | Description | Example |
|-----------|-------------|---------|
| `mcpServerId` | Server ID from registry | `"suse-bugzilla"` |
| `name` | Unique adapter name | `"my-bugzilla-adapter"` |
| `BUGZILLA_SERVER` | Bugzilla instance URL | `"https://bugzilla.suse.com"` |

### Environment Variables

The adapter accepts environment variables for Bugzilla configuration:
- **BUGZILLA_SERVER**: Your Bugzilla instance URL (e.g., `https://bugzilla.suse.com`)
- **BUGZILLA_API_KEY**: Your Bugzilla API key (optional, can be set via MCP client)

**Note**: The adapter will automatically deploy as a Docker sidecar container using the `kskarthik/mcp-bugzilla` image.

## Complete Example

```json
{
  "mcpServerId": "suse-bugzilla",
  "name": "suse-bugzilla-prod",
  "description": "Production Bugzilla adapter for SUSE issue tracking",
  "environmentVariables": {
    "BUGZILLA_SERVER": "https://bugzilla.suse.com"
  }
}
```

## Testing the Adapter

### 1. Verify Creation
```bash
# List your adapters (replace localhost:8913 with your registry URL)
curl -H "X-User-ID: your-user-id" http://your-registry-host:8913/api/v1/adapters
```

### 2. Test MCP Connectivity
```bash
# Get adapter details (replace localhost:8913 with your registry URL)
curl http://your-registry-host:8913/api/v1/adapters/{adapter-id}

# Test MCP endpoint (replace localhost:8911 with your proxy URL)
curl http://your-proxy-host:8911/api/v1/adapters/{adapter-id}/mcp
```

### 3. MCP Client Configuration

Configure your MCP client to connect to:
```
http://your-proxy-host:8911/api/v1/adapters/{adapter-id}/mcp
```

**IMPORTANT**: Include the following header in all MCP client requests:
```
X-User-ID: developer-user
```

Replace `your-proxy-host:8911` with your actual SUSE AI Universal Proxy URL.

## Available Bugzilla Operations

Once connected, the Bugzilla adapter provides:

- **Bug Search**: Find bugs by ID, status, assignee, etc.
- **Bug Creation**: Create new bug reports
- **Bug Updates**: Modify existing bugs (status, assignee, comments)
- **Bug History**: View bug change history
- **User Management**: Look up user information
- **Product/Component Queries**: Get available products and components

## Troubleshooting

### Common Issues

**"Adapter creation failed"**
- Verify your Bugzilla API key is valid
- Check the Bugzilla server URL is accessible
- Ensure the registry service is running

**"MCP connection failed"**
- Confirm the proxy service is running on port 8911
- Check the adapter ID is correct
- Verify the adapter status is "ready"
- **Ensure you're using the correct X-User-ID header: `X-User-ID: developer-user`**

**"Authentication failed"**
- Ensure your API key has the necessary permissions
- Check the BUGZILLA_SERVER environment variable is set correctly
- Verify the Bugzilla server accepts your API key

### Debug Commands

```bash
# Check registry health (replace localhost:8913 with your registry URL)
curl http://your-registry-host:8913/health

# Check proxy health (replace localhost:8911 with your proxy URL)
curl http://your-proxy-host:8911/health

# View adapter logs (if using Docker)
docker logs <container-name>

# Test Bugzilla API directly
curl -H "X-BUGZILLA-API-KEY: your-key" https://bugzilla.suse.com/rest/whoami
```

## Advanced Configuration

### Multiple Bugzilla Instances

Create separate adapters for different Bugzilla instances:

```bash
# SUSE Bugzilla
curl -X POST http://localhost:8913/api/v1/adapters \
  -H "Content-Type: application/json" \
  -H "X-User-ID: developer" \
  -d '{
    "mcpServerId": "suse-bugzilla",
    "name": "suse-bugzilla",
    "environmentVariables": {
      "BUGZILLA_SERVER": "https://bugzilla.suse.com"
    }
  }'

# Custom Bugzilla
curl -X POST http://localhost:8913/api/v1/adapters \
  -H "Content-Type: application/json" \
  -H "X-User-ID: developer" \
  -d '{
    "mcpServerId": "suse-bugzilla",
    "name": "custom-bugzilla",
    "environmentVariables": {
      "BUGZILLA_SERVER": "https://bugzilla.example.com"
    }
  }'
```

### User Permissions

The system supports user/group-based access control:

```bash
# Create a group for Bugzilla users
curl -X POST http://localhost:8913/api/v1/groups \
  -H "Content-Type: application/json" \
  -H "X-User-ID: admin" \
  -d '{
    "id": "bugzilla-users",
    "name": "Bugzilla Users",
    "permissions": ["server:suse-bugzilla:*"]
  }'

# Add user to group
curl -X POST http://localhost:8913/api/v1/groups/bugzilla-users/members \
  -H "Content-Type: application/json" \
  -H "X-User-ID: admin" \
  -d '{"userId": "developer"}'
```

## API Reference

### Adapter Endpoints

- `POST /api/v1/adapters` - Create adapter
- `GET /api/v1/adapters` - List adapters
- `GET /api/v1/adapters/{id}` - Get adapter details
- `PUT /api/v1/adapters/{id}` - Update adapter
- `DELETE /api/v1/adapters/{id}` - Delete adapter

### MCP Endpoints

- `POST /api/v1/adapters/{id}/mcp` - MCP JSON-RPC endpoint (requires `X-User-ID: developer-user` header)
- `GET /api/v1/adapters/{id}/health` - Adapter health check (requires `X-User-ID: developer-user` header)

## Next Steps

1. **Explore Capabilities**: Use MCP client to discover available Bugzilla operations
2. **Automate Workflows**: Create scripts for common Bugzilla operations
3. **Monitor Usage**: Check adapter logs and metrics
4. **Scale**: Create adapters for different teams/projects

For more information, visit the [Bugzilla MCP Server documentation](https://github.com/openSUSE/mcp-bugzilla).</content>
<parameter name="filePath">BUGZILLA_ADAPTER_README.md