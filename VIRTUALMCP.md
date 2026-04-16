# Virtual MCP Aggregator

The Virtual MCP Aggregator allows you to combine multiple MCP adapters into a single, unified endpoint. This is useful for exposing tools from different sources (e.g., Bugzilla, Uyuni, Airtable) as if they were coming from a single remote MCP server.

## Overview

A Virtual Adapter behaves exactly like a standard Remote MCP adapter:
- It has its own unique authentication token.
- It provides a single `/api/v1/mcp/{name}` endpoint.
- It automatically prefixes tools, resources, and prompts to avoid name collisions (e.g., `bugzilla__get_incident`).
- It can be used by any standard MCP client (Gemini, Claude Desktop, VSCode).

## Step 1: Create Source Adapters

First, create the adapters you want to aggregate. In this example, we'll create a Bugzilla adapter and a Uyuni adapter.

### Create Bugzilla Adapter
```bash
curl -X POST -H "Content-Type: application/json" \
     -H "X-User-ID: admin" \
     -d '{
       "mcpServerId": "bugzilla",
       "name": "bugzilla-source",
       "connectionType": "StreamableHttp",
       "environmentVariables": {
         "BUGZILLA_SERVER": "https://bugzilla.suse.com",
         "BUGZILLA_APIKEY": "your-api-key"
       }
     }' \
     http://localhost:8911/api/v1/adapters
```

### Create Uyuni Adapter
```bash
curl -X POST -H "Content-Type: application/json" \
     -H "X-User-ID: admin" \
     -d '{
       "mcpServerId": "uyuni",
       "name": "uyuni-source",
       "connectionType": "StreamableHttp",
       "environmentVariables": {
         "UYUNI_SERVER": "https://uyuni.example.com",
         "UYUNI_USER": "admin",
         "UYUNI_PASS": "secret"
       }
     }' \
     http://localhost:8911/api/v1/adapters
```

## Step 2: Create the Virtual Adapter

Now, create a Virtual Adapter that aggregates the two source adapters by setting `connectionType` to `"Virtual"` and providing a list of `sourceAdapters`.

```bash
curl -X POST -H "Content-Type: application/json" \
     -H "X-User-ID: admin" \
     -d '{
       "name": "my-unified-mcp",
       "connectionType": "Virtual",
       "sourceAdapters": ["bugzilla-source", "uyuni-source"]
     }' \
     http://localhost:8911/api/v1/adapters
```

### Response Example
The response will include the aggregated endpoint URL and a secure token:

```json
{
  "id": "my-unified-mcp",
  "mcpClientConfig": {
    "gemini": {
      "mcpServers": {
        "my-unified-mcp": {
          "headers": {
            "Authorization": "Bearer <secure-token>",
            "X-User-ID": "admin"
          },
          "httpUrl": "http://localhost:8911/api/v1/mcp/my-unified-mcp"
        }
      }
    }
  },
  "status": "ready"
}
```

## Step 3: Use the Virtual Adapter

You can now interact with the virtual adapter using standard MCP JSON-RPC requests.

### Get Tool List
Use the token returned in the previous step to list all tools from both adapters.

```bash
curl -X POST -H "Content-Type: application/json" \
     -H "Authorization: Bearer <secure-token>" \
     -H "X-User-ID: admin" \
     -d '{
       "jsonrpc": "2.0",
       "id": 1,
       "method": "tools/list",
       "params": {}
     }' \
     http://localhost:8911/api/v1/mcp/my-unified-mcp
```

### Example Result
Tools will be prefixed with the source adapter name followed by `__`:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "tools": [
      {
        "name": "bugzilla-source__get_incident",
        "description": "[bugzilla-source] Get incident details",
        "inputSchema": { ... }
      },
      {
        "name": "uyuni-source__list_systems",
        "description": "[uyuni-source] List systems in Uyuni",
        "inputSchema": { ... }
      }
    ]
  }
}
```

## Key Features

- **Isolation**: Each virtual adapter has its own token and permission set.
- **Flexibility**: You can create multiple virtual adapters with different combinations of source adapters for different teams or purposes.
- **Standard Protocol**: Since the endpoint follows the standard MCP HTTP protocol, it is compatible with all MCP-enabled AI clients.
