# SUSE AI Uniproxy Plugin System

SUSE AI Uniproxy supports an extensibility model where external services ("plugins") can register themselves dynamically to extend the proxy's capabilities. This allows for modular addition of features like new MCP server sources (VirtualMCP), smart agents, or registry providers without recompiling the core binary.

## Architecture

Plugins in SUSE AI Uniproxy are standalone HTTP services that:
1.  **Register** themselves with the Uniproxy upon startup.
2.  **Expose** required endpoints (Health checks, Discovery) that the Uniproxy consumes.
3.  **Heartbeat** via periodic health checks initiated by the Uniproxy.

## Plugin Types

Currently supported service types:

-   **`smartagents`**: Plugins that provide autonomous agent capabilities.
-   **`registry`**: External registry sources for MCP servers.
-   **`virtualmcp`**: Services that dynamically host or generate MCP servers. The Uniproxy will automatically discover MCP implementations exposed by these plugins.

## Developing a Plugin

To create a plugin, your service must implement a few required endpoints and perform a registration call.

### 1. Registration

When your plugin starts, it must register itself with the Uniproxy.

**Endpoint:** `POST {UNIPROXY_URL}/api/v1/plugins/register`

**Payload:**

```json
{
  "service_id": "my-plugin-service-id",
  "service_type": "virtualmcp", 
  "service_url": "http://my-plugin-host:8080",
  "version": "1.0.0",
  "capabilities": [
    {
      "path": "/v1/agents/*",
      "methods": ["GET", "POST"],
      "description": "Agent management endpoints"
    }
  ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `service_id` | string | Unique identifier for your service instance. |
| `service_type` | string | One of `smartagents`, `registry`, `virtualmcp`. |
| `service_url` | string | The base URL where your service is reachable by the Uniproxy. |
| `capabilities` | array | (Optional) List of API paths your service handles if it acts as a router extension. |

### 2. Required Endpoints

Your plugin service must expose the following endpoints:

#### Health Check
**URL:** `{service_url}/health`  
**Method:** `GET`  
**Response:** `200 OK`

The Uniproxy will periodically call this endpoint to ensure your plugin is active. If this fails, the plugin is marked as unhealthy.

### 3. VirtualMCP Specifics

If your `service_type` is `virtualmcp`, you must also implement the discovery endpoint. This allows the Uniproxy to "suck in" the MCP servers your plugin provides and register them in its central registry.

#### MCP Discovery
**URL:** `{service_url}/api/v1/mcps`  
**Method:** `GET`  
**Response:**

```json
{
  "service": "my-virtual-mcp-service",
  "count": 1,
  "implementations": [
    {
      "id": "server-1",
      "name": "My Generated Server",
      "description": "A server dynamically created by this plugin",
      "version": "0.1.0",
      "tools": [
        {
          "name": "calculate_sum",
          "description": "Adds two numbers",
          "input_schema": { ... }
        }
      ]
    }
  ]
}
```

When the Uniproxy detects a `virtualmcp` plugin, it will:
1.  Call `{service_url}/api/v1/mcps`.
2.  Parse the returned implementations.
3.  Register them as `virtualmcp` transport type MCP servers in the core registry.
4.  Clients can then connect to these servers via the Uniproxy standard adapters.

## Example Integration

### 1. Start your plugin service
Ensure your service is running (e.g., on `http://localhost:9000`) and exposing `/health`.

### 2. Register with Uniproxy
Assuming Uniproxy is running on `http://localhost:8911`:

```bash
curl -X POST http://localhost:8911/api/v1/plugins/register \
  -H "Content-Type: application/json" \
  -d '{
    "service_id": "demo-plugin-01",
    "service_type": "virtualmcp",
    "service_url": "http://localhost:9000",
    "version": "1.0.0"
  }'
```

### 3. Verify
Check that your service is listed:

```bash
curl http://localhost:8911/api/v1/plugins/services
```
