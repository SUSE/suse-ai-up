# Adapters Documentation

This document describes how to create and manage MCP (Model Context Protocol) adapters in the SUSE AI Universal Proxy system.

## Overview

Adapters provide the interface between MCP clients (like Claude Desktop, VSCode, or Gemini) and MCP servers running in sidecar containers. The system supports multiple connection types and automatic configuration generation.

## Adapter Types

### StreamableHttp Adapters

StreamableHttp adapters proxy MCP traffic through HTTP endpoints, supporting both stdio-based and containerized MCP servers.

**Characteristics:**
- HTTP-based communication
- Automatic sidecar deployment
- Multi-format client configuration (Gemini, VSCode)
- Environment variable templating support

### Remote Adapters

Remote adapters connect to externally hosted MCP servers without sidecar deployment.

**Characteristics:**
- Direct HTTP connection to remote servers
- No sidecar container management
- Authentication via API keys or tokens
- Limited to remote server capabilities

## Creating Adapters

### API Endpoint

```http
POST /api/v1/adapters
```

### Basic Request Structure

```json
{
  "mcpServerId": "server-name",
  "name": "my-adapter-name",
  "connectionType": "StreamableHttp",
  "protocol": "MCP",
  "environmentVariables": {
    "ENV_VAR_NAME": "value"
  }
}
```

### Parameters

- **`mcpServerId`** (string, required): ID of the server from the MCP registry
- **`name`** (string, required): Unique name for the adapter
- **`connectionType`** (string, required): `"StreamableHttp"` or `"Remote"`
- **`protocol`** (string, required): Protocol type (usually `"MCP"`)
- **`environmentVariables`** (object, optional): Key-value pairs of environment variables

## Variable Passing and Templating

### Environment Variables

Environment variables are passed to the adapter's sidecar container and can be used in templated commands.

```bash
curl -X POST -H "Content-Type: application/json" \
     -H "X-User-ID: admin" \
     -d '{
       "mcpServerId": "uyuni",
       "name": "my-uyuni-adapter",
       "connectionType": "StreamableHttp",
       "protocol": "MCP",
       "environmentVariables": {
         "UYUNI_SERVER": "https://uyuni.example.com",
         "UYUNI_USER": "admin",
         "UYUNI_PASS": "mypassword"
       }
     }' \
     http://localhost:8911/api/v1/adapters
```

### Template Variables

Registry commands can use template variables that reference the `config.secrets` section:

```yaml
# In registry
sidecarConfig:
  command: "docker run -it --rm {{uyuni.server}} {{uyuni.user}} {{uyuni.pass}}"

config:
  secrets:
    - env: UYUNI_SERVER
      name: uyuni.server
      templated: true
```

**Result:** `docker run -it --rm -e UYUNI_SERVER=https://uyuni.example.com -e UYUNI_USER=admin -e UYUNI_PASS=mypassword`

### Selective Variable Usage

You can choose which variables to provide:

```json
{
  "mcpServerId": "uyuni",
  "name": "uyuni-readonly",
  "environmentVariables": {
    "UYUNI_SERVER": "https://uyuni.example.com"
    // Only server, no user/pass for readonly access
  }
}
```

## Access Control (Groups/Users)

### Route Assignments

Adapters support group-based and user-based access control through route assignments:

```json
{
  "mcpServerId": "uyuni",
  "name": "uyuni-admin",
  "connectionType": "StreamableHttp",
  "protocol": "MCP",
  "routeAssignments": [
    {
      "group": "admin",
      "permissions": ["read", "write"]
    },
    {
      "user": "alice",
      "permissions": ["read"]
    }
  ]
}
```

### Permission Levels

- **`read`**: Can query the MCP server
- **`write`**: Can modify server state
- **`admin`**: Full administrative access

### Group-Based Access

```json
{
  "routeAssignments": [
    {
      "group": "developers",
      "permissions": ["read", "write"]
    }
  ]
}
```

### User-Based Access

```json
{
  "routeAssignments": [
    {
      "user": "alice@example.com",
      "permissions": ["read"]
    }
  ]
}
```

## Practical Examples

### Uyuni Server (Docker with Templates)

```bash
curl -X POST -H "Content-Type: application/json" \
     -H "X-User-ID: admin" \
     -d '{
       "mcpServerId": "uyuni",
       "name": "prod-uyuni",
       "connectionType": "StreamableHttp",
       "protocol": "MCP",
       "environmentVariables": {
         "UYUNI_SERVER": "https://uyuni.prod.company.com",
         "UYUNI_USER": "automation",
         "UYUNI_PASS": "secret-password"
       },
       "routeAssignments": [
         {
           "group": "devops",
           "permissions": ["read", "write"]
         }
       ]
     }' \
     http://localhost:8911/api/v1/adapters
```

**Generated command:** `docker run -it --rm -e UYUNI_SERVER=https://uyuni.prod.company.com -e UYUNI_USER=automation -e UYUNI_PASS=secret-password -e UYUNI_MCP_TRANSPORT=http -e UYUNI_MCP_HOST=0.0.0.0`

### Bugzilla (Python with Repository)

```bash
curl -X POST -H "Content-Type: application/json" \
     -H "X-User-ID: admin" \
     -d '{
       "mcpServerId": "bugzilla",
       "name": "bugzilla-tracker",
       "connectionType": "StreamableHttp",
       "protocol": "MCP",
       "environmentVariables": {
         "BUGZILLA_SERVER": "https://bugzilla.suse.com",
         "BUGZILLA_APIKEY": "my-api-key"
       }
     }' \
     http://localhost:8911/api/v1/adapters
```

**Process:**
1. Install uv and git
2. Clone `https://github.com/openSUSE/mcp-bugzilla`
3. Run `uv sync`
4. Execute: `uv run mcp-bugzilla --bugzilla-server https://bugzilla.suse.com --host 127.0.0.1 --port 8000`

### Airtable (NPX Package)

```bash
curl -X POST -H "Content-Type: application/json" \
     -H "X-User-ID: admin" \
     -d '{
       "mcpServerId": "airtable-mcp-server",
       "name": "airtable-integration",
       "connectionType": "StreamableHttp",
       "protocol": "MCP",
       "environmentVariables": {
         "AIRTABLE_API_KEY": "patABC123.def456ghi789jkl012mno345pqr678stu901vwx"
       }
     }' \
     http://localhost:8911/api/v1/adapters
```

**Generated command:** `npx -y @nekzus/npm-sentinel-mcp`

## Response Format

Successful adapter creation returns:

```json
{
  "id": "adapter-name",
  "mcpServerId": "server-name",
  "mcpClientConfig": {
    "gemini": {
      "mcpServers": {
        "adapter-name": {
          "headers": {"Authorization": "Bearer <token>"},
          "httpUrl": "http://localhost:8911/api/v1/adapters/adapter-name/mcp"
        }
      }
    },
    "vscode": {
      "inputs": [],
      "servers": {
        "adapter-name": {
          "headers": {"Authorization": "Bearer <token>"},
          "type": "http",
          "url": "http://localhost:8911/api/v1/adapters/adapter-name/mcp"
        }
      }
    }
  },
  "capabilities": {...},
  "status": "ready",
  "createdAt": "2026-01-12T09:15:30.123Z"
}
```

## Client Configuration

### Gemini Format

Use the `gemini` section for Claude Desktop or Gemini clients:

```json
{
  "mcpServers": {
    "my-adapter": {
      "headers": {
        "Authorization": "Bearer <session-token>"
      },
      "httpUrl": "http://localhost:8911/api/v1/adapters/my-adapter/mcp"
    }
  }
}
```

### VSCode Format

Use the `vscode` section for VSCode MCP extension:

```json
{
  "inputs": [],
  "servers": {
    "my-adapter": {
      "headers": {
        "Authorization": "Bearer <session-token>"
      },
      "type": "http",
      "url": "http://localhost:8911/api/v1/adapters/my-adapter/mcp"
    }
  }
}
```

## Adapter Management

### List Adapters

```bash
curl -H "X-User-ID: admin" http://localhost:8911/api/v1/adapters
```

### Delete Adapter

```bash
curl -X DELETE -H "X-User-ID: admin" \
     http://localhost:8911/api/v1/adapters/adapter-name
```

### Adapter Status

Adapters can have these statuses:
- **`ready`**: Successfully deployed and running
- **`deploying`**: Sidecar container being created
- **`error`**: Deployment failed or container has errors
- **`stopped`**: Manually stopped

## Troubleshooting

### Common Issues

#### Template Variables Not Resolved
- Check that secrets have `templated: true`
- Verify environment variables are provided in the request
- Check logs for template processing errors

#### Sidecar Deployment Failures
- Check Kubernetes cluster connectivity
- Verify image availability and registry access
- Check pod logs for container startup errors

#### Authentication Issues
- Verify `X-User-ID` header is provided
- Check route assignments for access permissions
- Ensure user/group has appropriate permissions

#### Connection Refused
- Check if sidecar pod is running: `kubectl get pods -n suse-ai-up-mcp`
- Verify port configuration in registry
- Check pod logs for application errors

### Debug Commands

Check adapter status:
```bash
curl -H "X-User-ID: admin" http://localhost:8911/api/v1/adapters/adapter-name
```

Check sidecar logs:
```bash
kubectl logs -n suse-ai-up-mcp <sidecar-pod-name>
```

Check service logs:
```bash
kubectl logs -n suseai deployment/suse-ai-up
```

Validate registry:
```bash
curl -H "X-User-ID: admin" http://localhost:8911/api/v1/registry
```</content>
<parameter name="filePath">ADAPTERS.md