# SUSE AI Uniproxy Authentication

## Overview

SUSE AI Uniproxy supports multiple authentication methods to secure access to MCP servers and adapters. Authentication can be configured at the adapter level, allowing different security requirements for different MCP servers.

## Authentication Types

### Bearer Token Authentication

The most common authentication method using JWT or API tokens.

**Configuration:**
```json
{
  "authentication": {
    "required": true,
    "type": "bearer",
    "bearerToken": {
      "token": "your-secret-token",
      "dynamic": false,
      "expiresAt": "2025-12-31T23:59:59Z"
    }
  }
}
```

**Client Usage:**
```bash
curl -H "Authorization: Bearer your-secret-token" \
     http://localhost:8911/api/v1/adapters/my-adapter/tools
```

### Basic Authentication

HTTP Basic authentication using username/password.

**Configuration:**
```json
{
  "authentication": {
    "required": true,
    "type": "basic",
    "basic": {
      "username": "admin",
      "password": "secret-password"
    }
  }
}
```

**Client Usage:**
```bash
curl -u "admin:secret-password" \
     http://localhost:8911/api/v1/adapters/my-adapter/tools
```

### API Key Authentication

API key authentication via header, query parameter, or cookie.

**Configuration:**
```json
{
  "authentication": {
    "required": true,
    "type": "apikey",
    "apiKey": {
      "key": "your-api-key",
      "name": "X-API-Key",
      "location": "header"
    }
  }
}
```

**Client Usage:**
```bash
# Header
curl -H "X-API-Key: your-api-key" \
     http://localhost:8911/api/v1/adapters/my-adapter/tools

# Query parameter
curl "http://localhost:8911/api/v1/adapters/my-adapter/tools?api_key=your-api-key"

# Cookie
curl -b "api_key=your-api-key" \
     http://localhost:8911/api/v1/adapters/my-adapter/tools
```

## Authentication Flow

### 1. Adapter Creation
When creating an adapter, specify authentication requirements:

```bash
curl -X POST http://localhost:8911/api/v1/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "secure-mcp-server",
    "connectionType": "RemoteHttp",
    "apiBaseUrl": "https://api.example.com/mcp",
    "authentication": {
      "required": true,
      "type": "bearer",
      "bearerToken": {
        "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
        "dynamic": false
      }
    }
  }'
```

### 2. Authentication Validation
All requests to authenticated adapters are validated:

```bash
# This will fail without proper authentication
curl http://localhost:8911/api/v1/adapters/secure-mcp-server/tools
# Returns: {"error": "Authentication required"}

# This will succeed with proper authentication
curl -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
     http://localhost:8911/api/v1/adapters/secure-mcp-server/tools
```

### 3. Token Management
For adapters with token management capabilities:

```bash
# Get current token
curl http://localhost:8911/api/v1/adapters/my-adapter/token

# Validate token
curl -X POST http://localhost:8911/api/v1/adapters/my-adapter/token/validate \
  -H "Authorization: Bearer current-token"

# Refresh token
curl -X POST http://localhost:8911/api/v1/adapters/my-adapter/token/refresh \
  -H "Authorization: Bearer current-token"
```

## VirtualMCP Authentication

VirtualMCP adapters have special authentication handling:

```json
{
  "name": "virtual-mcp-adapter",
  "connectionType": "LocalStdio",
  "authentication": {
    "required": true,
    "type": "bearer",
    "bearerToken": {
      "token": "virtualmcp-token",
      "dynamic": false,
      "expiresAt": "2025-12-31T23:59:59Z"
    }
  },
  "mcpClientConfig": {
    "mcpServers": {
      "virtualmcp": {
        "command": "tsx",
        "args": ["templates/virtualmcp-server.ts"],
        "env": {
          "SERVER_NAME": "virtual-mcp-adapter",
          "TOOLS_CONFIG": "[...]",
          "API_BASE_URL": "https://api.example.com"
        }
      }
    }
  }
}
```

## Security Best Practices

### 1. Use HTTPS in Production
Always enable TLS for production deployments:

```bash
export AUTH_MODE=production
export TLS_CERT_FILE=/path/to/cert.pem
export TLS_KEY_FILE=/path/to/key.pem
```

### 2. Token Rotation
Implement regular token rotation:

```bash
# Update adapter with new token
curl -X PUT http://localhost:8911/api/v1/adapters/my-adapter \
  -H "Content-Type: application/json" \
  -d '{
    "authentication": {
      "bearerToken": {
        "token": "new-rotated-token"
      }
    }
  }'
```

### 3. Least Privilege
Configure minimal required permissions for each adapter.

### 4. Monitor Authentication
Check authentication logs and metrics:

```bash
# View authentication logs
curl http://localhost:8911/api/v1/monitoring/logs

# Check authentication metrics
curl http://localhost:8911/api/v1/monitoring/metrics
```

## Error Responses

### Authentication Required
```json
{
  "error": "Authentication required: missing Authorization header"
}
```

### Invalid Credentials
```json
{
  "error": "Authentication required: invalid token"
}
```

### Expired Token
```json
{
  "error": "Authentication required: token expired"
}
```

### Insufficient Permissions
```json
{
  "error": "Forbidden: insufficient permissions"
}
```

## Configuration Examples

### OAuth2 Integration
```json
{
  "authentication": {
    "required": true,
    "type": "bearer",
    "bearerToken": {
      "token": "${OAUTH2_ACCESS_TOKEN}",
      "dynamic": true,
      "expiresAt": "${OAUTH2_EXPIRES_AT}"
    }
  }
}
```

### Multi-Factor Authentication
```json
{
  "authentication": {
    "required": true,
    "type": "apikey",
    "apiKey": {
      "key": "${MFA_API_KEY}",
      "name": "X-MFA-Token",
      "location": "header"
    }
  }
}
```

### Service Account Authentication
```json
{
  "authentication": {
    "required": true,
    "type": "basic",
    "basic": {
      "username": "${SERVICE_ACCOUNT_USER}",
      "password": "${SERVICE_ACCOUNT_PASSWORD}"
    }
  }
}
```