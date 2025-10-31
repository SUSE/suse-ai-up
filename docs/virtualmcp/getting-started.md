# SUSE AI Virtual MCP Service - Getting Started

This guide will help you get started with the SUSE AI Virtual MCP Service, which allows you to create MCP servers from existing APIs and databases without writing any code.

## Prerequisites

### System Requirements
- SUSE AI Universal Proxy running and accessible
- Access to OpenAPI schemas or database connections
- API keys or database credentials for your source systems

### Required Components
- **SUSE AI Universal Proxy**: The main proxy service that Virtual MCP integrates with
- **Source Systems**: OpenAPI-compliant APIs or databases you want to expose as MCP servers

## Installation

### 1. Verify Proxy Service
Ensure the SUSE AI Universal Proxy is running and accessible:

```bash
# Check proxy health
curl http://localhost:8911/health

# Verify plugin services are available
curl http://localhost:8911/plugins/services
```

### 2. Confirm Virtual MCP Service Registration
The Virtual MCP service should be automatically registered with the proxy. Verify it's available:

```bash
curl http://localhost:8911/plugins/services | jq '.services[] | select(.service_type == "virtualmcp")'
```

## Creating Your First Virtual MCP Server

### From OpenAPI Schema

#### Step 1: Prepare Your OpenAPI Schema
Ensure you have access to an OpenAPI specification file (JSON or YAML format). For this example, we'll use the Petstore API:

```bash
# Download a sample OpenAPI schema
curl -o petstore.json https://petstore.swagger.io/v2/swagger.json

# Or use your own API schema
# curl -o my-api.json https://my-api.example.com/openapi.json
```

#### Step 2: Create Virtual MCP Configuration
Create a configuration file for your Virtual MCP server:

```json
{
  "name": "petstore-api",
  "type": "openapi",
  "source": {
    "url": "https://petstore.swagger.io/v2/swagger.json",
    "format": "openapi-v2"
  },
  "generation": {
    "tools": [
      {
        "path": "/pets",
        "methods": ["GET", "POST"],
        "tool_name": "manage_pets"
      },
      {
        "path": "/pets/{id}",
        "methods": ["GET", "PUT", "DELETE"],
        "tool_name": "manage_pet"
      }
    ],
    "resources": [
      {
        "path": "/pets/{id}",
        "resource_name": "pet_details"
      }
    ]
  },
  "authentication": {
    "type": "bearer",
    "token_env": "PETSTORE_API_TOKEN"
  }
}
```

#### Step 3: Generate the MCP Server
Submit the configuration to create your Virtual MCP server:

```bash
curl -X POST http://localhost:8911/virtualmcp/generate \
  -H "Content-Type: application/json" \
  -d @petstore-config.json
```

**Response:**
```json
{
  "server_id": "virtualmcp-petstore-api-12345",
  "status": "generating",
  "message": "Virtual MCP server generation started",
  "estimated_completion": "30s"
}
```

#### Step 4: Check Generation Status
Monitor the generation progress:

```bash
# Check status
curl http://localhost:8911/virtualmcp/status/virtualmcp-petstore-api-12345

# Response when complete
{
  "server_id": "virtualmcp-petstore-api-12345",
  "status": "ready",
  "mcp_endpoint": "http://localhost:8911/virtualmcp/servers/virtualmcp-petstore-api-12345/mcp",
  "tools_count": 3,
  "resources_count": 1
}
```

#### Step 5: Test Your Virtual MCP Server
Test the generated MCP server:

```bash
# Initialize MCP connection
curl -X POST http://localhost:8911/virtualmcp/servers/virtualmcp-petstore-api-12345/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {},
      "clientInfo": {
        "name": "test-client",
        "version": "1.0.0"
      }
    }
  }'
```

### From Database Schema

#### Step 1: Prepare Database Connection
Ensure you have database connection details and appropriate permissions:

```bash
# Example PostgreSQL connection details
export DB_HOST="localhost"
export DB_PORT="5432"
export DB_NAME="analytics"
export DB_USER="mcp_user"
export DB_PASSWORD="secure_password"
```

#### Step 2: Create Database Configuration
Create a configuration file for database-based Virtual MCP server:

```json
{
  "name": "company-analytics-db",
  "type": "database",
  "source": {
    "connection": {
      "type": "postgresql",
      "host_env": "DB_HOST",
      "port_env": "DB_PORT",
      "database_env": "DB_NAME",
      "username_env": "DB_USER",
      "password_env": "DB_PASSWORD"
    },
    "ssl_mode": "require"
  },
  "generation": {
    "tables": [
      {
        "name": "users",
        "tools": ["select", "count"],
        "description": "User account information"
      },
      {
        "name": "orders",
        "tools": ["select", "analytics"],
        "description": "Order transaction data"
      }
    ],
    "queries": [
      {
        "name": "monthly_revenue",
        "sql": "SELECT DATE_TRUNC('month', created_at) as month, SUM(amount) as revenue FROM orders WHERE created_at >= $1 GROUP BY month ORDER BY month DESC",
        "parameters": [
          {
            "name": "start_date",
            "type": "date",
            "description": "Start date for revenue calculation"
          }
        ],
        "description": "Calculate monthly revenue from orders"
      }
    ]
  },
  "security": {
    "row_level_security": true,
    "audit_logging": true
  }
}
```

#### Step 3: Generate Database MCP Server
Submit the database configuration:

```bash
curl -X POST http://localhost:8911/virtualmcp/generate \
  -H "Content-Type: application/json" \
  -d @database-config.json
```

#### Step 4: Test Database Tools
Once generated, test the database tools:

```bash
# List available tools
curl -X POST http://localhost:8911/virtualmcp/servers/virtualmcp-company-analytics-db-67890/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "mcp-session-id: YOUR_SESSION_ID" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/list"
  }'
```

## Advanced Configuration

### Authentication Setup

#### API Key Authentication
```json
{
  "authentication": {
    "type": "bearer",
    "token_env": "API_TOKEN"
  }
}
```

#### OAuth 2.0 Authentication
```json
{
  "authentication": {
    "type": "oauth2",
    "token_url": "https://auth.example.com/oauth2/token",
    "client_id_env": "OAUTH_CLIENT_ID",
    "client_secret_env": "OAUTH_CLIENT_SECRET",
    "scopes": ["read", "write"]
  }
}
```

#### Database Authentication
```json
{
  "source": {
    "connection": {
      "type": "postgresql",
      "authentication": {
        "method": "iam",
        "role_arn": "arn:aws:iam::123456789012:role/mcp-database-role"
      }
    }
  }
}
```

### Tool Customization

#### Custom Tool Mapping
```json
{
  "generation": {
    "tools": [
      {
        "path": "/api/v1/users",
        "methods": ["GET"],
        "tool_name": "list_users",
        "description": "Retrieve a list of all users",
        "parameters": [
          {
            "name": "limit",
            "type": "integer",
            "description": "Maximum number of users to return",
            "default": 100
          }
        ]
      }
    ]
  }
}
```

#### Database Query Tools
```json
{
  "generation": {
    "queries": [
      {
        "name": "user_activity_report",
        "sql": "SELECT u.name, COUNT(o.id) as order_count, SUM(o.amount) as total_spent FROM users u LEFT JOIN orders o ON u.id = o.user_id WHERE u.created_at >= $1 GROUP BY u.id, u.name ORDER BY total_spent DESC",
        "parameters": [
          {
            "name": "registration_cutoff",
            "type": "date",
            "description": "Only include users registered after this date"
          }
        ],
        "result_format": "table"
      }
    ]
  }
}
```

## Integration with AI Agents

### Using with Smart Agents

#### 1. Create an Agent Configuration
```bash
curl -X POST http://localhost:8910/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "data-analyst",
    "supervisor": {
      "provider": "openai",
      "model": "gpt-4",
      "api": "sk-your_openai_key"
    },
    "worker": {
      "provider": "ollama",
      "model": "llama3.2:latest"
    },
    "mcp_servers": [
      "virtualmcp-company-analytics-db-67890"
    ]
  }'
```

#### 2. Test AI-Powered Database Queries
```bash
curl -X POST http://localhost:8910/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "data-analyst",
    "messages": [
      {
        "role": "user",
        "content": "What was the total revenue for last month, and who were our top 5 customers by spending?"
      }
    ]
  }'
```

## Monitoring and Management

### Check Server Status
```bash
# List all virtual MCP servers
curl http://localhost:8911/virtualmcp/servers

# Get specific server details
curl http://localhost:8911/virtualmcp/servers/virtualmcp-petstore-api-12345
```

### Update Server Configuration
```bash
curl -X PUT http://localhost:8911/virtualmcp/servers/virtualmcp-petstore-api-12345 \
  -H "Content-Type: application/json" \
  -d '{
    "authentication": {
      "token_env": "NEW_API_TOKEN"
    }
  }'
```

### Delete Virtual MCP Server
```bash
curl -X DELETE http://localhost:8911/virtualmcp/servers/virtualmcp-petstore-api-12345
```

## Troubleshooting

### Common Issues

#### Schema Validation Errors
```bash
# Check schema validation
curl http://localhost:8911/virtualmcp/validate \
  -H "Content-Type: application/json" \
  -d @your-config.json
```

#### Connection Failures
```bash
# Test API connectivity
curl -I https://your-api.example.com/health

# Test database connection
curl -X POST http://localhost:8911/virtualmcp/test-connection \
  -H "Content-Type: application/json" \
  -d @database-config.json
```

#### Generation Timeouts
```bash
# Check generation logs
curl http://localhost:8911/virtualmcp/logs/virtualmcp-server-id-12345

# Increase timeout for large schemas
curl -X POST http://localhost:8911/virtualmcp/generate \
  -H "Content-Type: application/json" \
  -H "X-Timeout: 300" \
  -d @large-config.json
```

### Performance Optimization

#### Caching Configuration
```json
{
  "performance": {
    "caching": {
      "enabled": true,
      "ttl_seconds": 300,
      "max_size_mb": 100
    }
  }
}
```

#### Connection Pooling
```json
{
  "source": {
    "connection": {
      "pool": {
        "max_connections": 10,
        "max_idle_time": "5m"
      }
    }
  }
}
```

## Next Steps

### Explore Advanced Features
- **Custom Tool Development**: Create specialized tools beyond auto-generated ones
- **Webhook Integration**: Set up real-time data push capabilities
- **Multi-Source Aggregation**: Combine multiple APIs into unified MCP servers
- **Performance Monitoring**: Set up comprehensive monitoring and alerting

### Production Deployment
- **Security Hardening**: Configure proper authentication and authorization
- **High Availability**: Set up load balancing and failover
- **Backup and Recovery**: Implement backup strategies for configurations
- **Monitoring**: Integrate with your monitoring stack

### Integration Examples
- **VS Code Integration**: Connect generated MCP servers to VS Code
- **Custom AI Applications**: Build AI apps that leverage your existing APIs
- **Data Analysis Workflows**: Create AI-powered data analysis pipelines

The Virtual MCP service provides a powerful way to bridge your existing infrastructure with modern AI capabilities, enabling seamless integration without code changes.