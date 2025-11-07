# OAuth 2.1 Compliant Token Management Implementation

## Overview

This implementation addresses the "INVALID_TOKEN" errors when using MCP Inspector with registered adapters by providing OAuth 2.1 compliant token management with proper token distribution mechanisms.

## Problems Solved

### 1. Token Generation Without Distribution
- **Before**: Discovery service auto-generated secure Bearer tokens but never provided them to clients
- **After**: Tokens are returned in registration responses and available via dedicated token APIs

### 2. Simplistic Validation
- **Before**: Basic string comparison (`token != adapter.Authentication.Token`)
- **After**: OAuth 2.1 compliant JWT validation with audience, expiration, and signature verification

### 3. Specification Non-Compliance
- **Before**: Non-standard token handling
- **After**: Proper JWT format with standard claims (jti, sub, aud, iss, iat, exp, scope)

## Implementation Details

### Core Components

#### 1. Token Manager (`pkg/auth/token_manager.go`)
- **JWT-based token generation** with RSA-256 signing
- **OAuth 2.1 compliant claims** including audience validation
- **Token lifecycle management** with creation, validation, and refresh
- **Refresh token support** for long-lived sessions

#### 2. Enhanced Adapter Middleware (`pkg/auth/adapter_middleware.go`)
- **Dual validation modes**: JWT validation with legacy fallback
- **Proper error handling** with standardized error codes
- **Audit logging** for security monitoring

#### 3. Token Handler (`internal/handlers/token_handler.go`)
- **Token retrieval API**: `GET /api/v1/adapters/{name}/token`
- **Token validation API**: `GET /api/v1/adapters/{name}/token/validate`
- **Token refresh API**: `POST /api/v1/adapters/{name}/token/refresh`

#### 4. Enhanced Discovery Service (`internal/service/discovery.go`)
- **Automatic security enhancement** for high-risk servers
- **JWT token generation** with proper token distribution
- **Token information in responses** for client configuration

### API Endpoints

#### Get Adapter Token
```http
GET /api/v1/adapters/{name}/token?generate=true&expiresIn=24
```

**Response (JWT format):**
```json
{
  "adapter": "my-mcp-adapter",
  "token": {
    "token_id": "abc123...",
    "access_token": "eyJhbGciOiJSUzI1NiIs...",
    "token_type": "Bearer",
    "expires_at": "2025-11-07T12:00:00Z",
    "issued_at": "2025-11-06T12:00:00Z",
    "scope": "mcp:read mcp:write",
    "audience": "http://localhost:8080",
    "issuer": "http://localhost:8911",
    "subject": "my-mcp-adapter",
    "format": "jwt"
  },
  "message": "New JWT token generated and saved to adapter"
}
```

#### Validate Token
```http
GET /api/v1/adapters/{name}/token/validate?token=eyJhbGciOiJSUzI1NiIs...
```

**Response:**
```json
{
  "valid": true,
  "adapter": "my-mcp-adapter",
  "token": {
    "token_id": "abc123...",
    "token_type": "Bearer",
    "expires_at": "2025-11-07T12:00:00Z",
    "format": "jwt"
  },
  "message": "Token is valid"
}
```

#### Refresh Token
```http
POST /api/v1/adapters/{name}/token/refresh?expiresIn=24
```

### Enhanced Discovery Registration

When registering high-risk servers, the response now includes token information:

```json
{
  "message": "Adapter successfully created from discovered server",
  "discoveredServer": { ... },
  "adapter": { ... },
  "security": {
    "enhanced": true,
    "auth_type": "bearer",
    "token_required": true,
    "note": "High-risk server automatically secured with bearer authentication",
    "token_info": {
      "token_id": "abc123...",
      "token_type": "Bearer",
      "expires_at": "2025-11-07T12:00:00Z",
      "issued_at": "2025-11-06T12:00:00Z",
      "scope": "mcp:read mcp:write",
      "note": "Save this token information for MCP Inspector client configuration"
    }
  }
}
```

## Usage Examples

### 1. MCP Inspector Client Configuration

After adapter registration or token retrieval:

```javascript
// Using the token from API response
const adapterConfig = {
  name: "my-mcp-adapter",
  baseUrl: "http://localhost:8911/api/v1/adapters/my-mcp-adapter",
  authentication: {
    type: "bearer",
    token: "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..." // JWT token from API
  }
};

// MCP Inspector can now authenticate successfully
const response = await fetch('/api/v1/adapters/my-mcp-adapter/mcp', {
  method: 'POST',
  headers: {
    'Authorization': `Bearer ${adapterConfig.authentication.token}`,
    'Content-Type': 'application/json'
  },
  body: JSON.stringify({ /* MCP request */ })
});
```

### 2. Automatic Token Refresh

```javascript
// Check if token needs refresh
async function ensureValidToken(adapterName) {
  const validateResponse = await fetch(`/api/v1/adapters/${adapterName}/token/validate?token=${currentToken}`);
  const validationResult = await validateResponse.json();
  
  if (!validationResult.valid) {
    // Refresh the token
    const refreshResponse = await fetch(`/api/v1/adapters/${adapterName}/token/refresh`, {
      method: 'POST'
    });
    const refreshResult = await refreshResponse.json();
    return refreshResult.token.access_token;
  }
  
  return currentToken;
}
```

### 3. High-Risk Server Registration

```bash
# Register a discovered server (high-risk servers get auto-secured)
curl -X POST http://localhost:8911/api/v1/register \
  -H "Content-Type: application/json" \
  -d '{
    "name": "risky-mcp-server",
    "address": "http://192.168.1.100:8080",
    "protocol": "mcp",
    "connection": "streamable_http",
    "vulnerability_score": "high",
    "metadata": {
      "auth_type": "none"
    }
  }'

# Response includes JWT token for immediate use
```

## Security Features

### 1. JWT Token Security
- **RSA-256 signing** with 2048-bit keys
- **Audience validation** prevents token reuse across adapters
- **Expiration enforcement** with configurable TTL
- **Unique token IDs** for audit and revocation

### 2. Token Lifecycle Management
- **Automatic expiration** (default 24 hours)
- **Refresh token support** (30-day validity)
- **Token rotation** on refresh requests
- **Audit logging** of all token operations

### 3. Fallback Compatibility
- **Legacy token support** for existing adapters
- **Gradual migration** path from legacy to JWT
- **Backward compatibility** with existing clients

## Configuration

### Environment Variables
- `PROXY_BASE_URL`: Base URL for token issuer (default: http://localhost:8911)
- `AUTH_MODE`: Authentication mode (development/oauth)

### Token Configuration
- **Default expiration**: 24 hours (configurable via API)
- **Refresh token validity**: 30 days
- **Signing algorithm**: RSA-256
- **Key size**: 2048 bits

## Migration Guide

### For Existing Adapters with Legacy Tokens

1. **Retrieve current token**:
   ```http
   GET /api/v1/adapters/{name}/token
   ```

2. **Generate new JWT token**:
   ```http
   GET /api/v1/adapters/{name}/token?generate=true
   ```

3. **Update client configuration** with new JWT token

### For New Adapters

1. **Register adapter** (high-risk servers get auto-secured)
2. **Use token from registration response** or retrieve via API
3. **Implement token refresh** in client for long-running sessions

## Testing

### Run Token Manager Tests
```bash
go test ./pkg/auth/...
```

### Test API Endpoints
```bash
# Build and start service
go build -o service ./cmd/service
./service

# Test token generation
  curl "http://localhost:8911/api/v1/adapters/test-adapter/token?generate=true"

# Test token validation
  curl "http://localhost:8911/api/v1/adapters/test-adapter/token/validate?token=YOUR_JWT_TOKEN"
```

## Benefits

1. **Eliminates INVALID_TOKEN errors** through proper token distribution
2. **OAuth 2.1 compliance** with industry-standard JWT tokens
3. **Enhanced security** with audience validation and expiration
4. **Backward compatibility** with existing legacy tokens
5. **Comprehensive APIs** for token lifecycle management
6. **Automatic security** for high-risk discovered servers
7. **Audit capabilities** with structured logging and token tracking

This implementation provides a robust, standards-compliant solution to the authentication challenges while maintaining compatibility with existing systems.