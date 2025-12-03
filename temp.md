# Complete Example: Create Adapter with Tools

Here's a working example of curl commands to create a VirtualMCP adapter with tools:

## Step 1: Upload VirtualMCP Server with Tools

```bash
curl -X POST http://192.168.64.17:8911/api/v1/registry/upload \
  -H "Content-Type: application/json" \
  -d '{
    "id": "weather-calculator-server",
    "name": "Weather & Calculator Server",
    "description": "A VirtualMCP server with weather and calculation tools",
    "version": "1.0.0",
    "validation_status": "new",
    "_meta": {
      "source": "virtualmcp"
    },
    "tools": [
      {
        "name": "get_weather",
        "description": "Get current weather for a location",
        "input_schema": {
          "type": "object",
          "properties": {
            "location": {
              "type": "string",
              "description": "City name or location"
            },
            "unit": {
              "type": "string",
              "enum": ["celsius", "fahrenheit"],
              "description": "Temperature unit",
              "default": "celsius"
            }
          },
          "required": ["location"]
        }
      },
      {
        "name": "calculate",
        "description": "Perform mathematical calculations",
        "input_schema": {
          "type": "object",
          "properties": {
            "expression": {
              "type": "string",
              "description": "Mathematical expression to evaluate (e.g., '\''2 + 2 * 3'\'')"
            }
          },
          "required": ["expression"]
        }
      },
      {
        "name": "get_time",
        "description": "Get current time for a timezone",
        "input_schema": {
          "type": "object",
          "properties": {
            "timezone": {
              "type": "string",
              "description": "Timezone (e.g., '\''America/New_York'\'', '\''Europe/London'\'')",
              "default": "UTC"
            }
          }
        }
      }
    ]
  }'
```

## Step 2: Create Adapter from the Server

```bash
curl -X POST http://192.168.64.17:8911/api/v1/registry/weather-calculator-server/create-adapter \
  -H "Content-Type: application/json" \
  -d '{
    "replicaCount": 1,
    "environmentVariables": {
      "WEATHER_API_KEY": "your-weather-api-key",
      "CALCULATOR_PRECISION": "4",
      "LOG_LEVEL": "info"
    }
  }'
```

## Expected Response

The second command will return an adapter configuration like:

```json
{
  "message": "VirtualMCP adapter created and deployed successfully",
  "adapter": {
    "name": "virtualmcp-weather-calculator-server",
    "protocol": "MCP",
    "connectionType": "StreamableHttp",
    "remoteUrl": "http://localhost:9000",
    "authentication": {
      "required": true,
      "type": "bearer",
      "bearerToken": {
        "token": "generated-token-here",
        "expiresAt": "2025-12-03T..."
      }
    },
    "mcpFunctionality": {
      "tools": [
        {
          "name": "get_weather",
          "description": "Get current weather for a location"
        },
        {
          "name": "calculate",
          "description": "Perform mathematical calculations"
        },
        {
          "name": "get_time",
          "description": "Get current time for a timezone"
        }
      ]
    }
  },
  "mcp_endpoint": "http://localhost:8911/api/v1/adapters/virtualmcp-weather-calculator-server/mcp",
  "token_info": {
    "token": "generated-token-here",
    "tokenType": "Bearer"
  }
}
```

## Key Points

- **Step 1** creates the server definition in the registry with tool schemas
- **Step 2** deploys the server and creates an authenticated adapter
- The adapter will have **3 tools** available: `get_weather`, `calculate`, and `get_time`
- Each tool has proper input validation schemas
- Environment variables can be passed for server configuration
- The adapter gets a unique bearer token for authentication

This creates a fully functional VirtualMCP adapter that you can use with MCP clients! 🚀</content>
<parameter name="filePath">temp.md