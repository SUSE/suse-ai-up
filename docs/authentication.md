# Authentication Guide

The SUSE AI Universal Proxy supports multiple authentication methods to secure access to MCP servers and protect sensitive AI operations. This guide covers the available authentication mechanisms, configuration options, and best practices for securing your proxy deployment.

## Overview

The proxy supports the following authentication methods:

- **Bearer Token Authentication**: Simple token-based authentication using JWT
- **OAuth 2.0/2.1**: Industry-standard OAuth flows with external providers
- **Development Mode**: Simplified authentication for development environments
- **No Authentication**: For public or internal services (not recommended for production)

## Authentication Architecture

### Service-Level Authentication

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   CLIENT    │───►│   PROXY     │───►│ MCP SERVER  │
│             │    │ (Auth Layer)│    │             │
└─────────────┘    └──────┬──────┘    └─────────────┘
                          │
                          ▼
                   ┌─────────────┐
                   │ AUTH CONFIG │
                   │ (per adapter)│
                   └─────────────┘
```

### Authentication Flow

1. **Client Request**: Client sends request with authentication credentials
2. **Proxy Validation**: Proxy validates credentials against configured auth method
3. **Token Processing**: Valid tokens are processed and user context is established
4. **Request Forwarding**: Authenticated request is forwarded to MCP server
5. **Response**: Server response is returned to authenticated client

## Bearer Token Authentication

Bearer token authentication uses JSON Web Tokens (JWT) for secure, stateless authentication.

### Configuration

```yaml
# Environment variables
AUTH_MODE=bearer
BEARER_TOKEN=your-jwt-token
TOKEN_ISSUER=suse-ai-up
TOKEN_AUDIENCE=proxy-service

# Or auto-generate tokens
AUTO_GENERATE_BEARER=true
TOKEN_EXPIRES_HOURS=24
```

### Token Generation

#### Manual Token Creation

```bash
# Generate a JWT token
curl -X POST http://localhost:8911/api/v1/auth/token \
  -H "Content-Type: application/json" \
  -d '{
    "adapter": "weather-adapter",
    "audience": "http://weather-server:8080",
    "expiresIn": 24
  }'
```

Response:
```json
{
  "tokenId": "tok_1733313600123456789",
  "accessToken": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
  "tokenType": "Bearer",
  "expiresAt": "2025-12-05T12:00:00Z",
  "issuedAt": "2025-12-04T12:00:00Z",
  "audience": "http://weather-server:8080",
  "issuer": "suse-ai-up",
  "subject": "weather-adapter"
}
```

#### Auto-Generated Tokens

```yaml
bearerConfig:
  autoGenerate: true
  expiresIn: 24  # hours
```

### Token Structure

JWT tokens contain the following claims:

```json
{
  "jti": "unique-token-id",
  "sub": "adapter-name",
  "aud": "target-audience",
  "iss": "suse-ai-up",
  "iat": 1733313600,
  "exp": 1733400000,
  "scope": "read write"
}
```

### Using Bearer Tokens

```bash
# Include token in Authorization header
curl -H "Authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..." \
  http://localhost:8080/mcp/tools
```

## OAuth 2.0/2.1 Authentication

OAuth authentication integrates with external identity providers for enterprise-grade authentication.

### Supported Providers

- **Azure AD**: Microsoft identity platform
- **Google**: Google Identity services
- **Okta**: Enterprise identity management
- **Auth0**: Universal identity platform
- **Keycloak**: Open-source identity provider
- **Custom**: Any OAuth 2.0/2.1 compliant provider

### Configuration

```yaml
oauth:
  provider: "azure"
  clientId: "your-client-id"
  clientSecret: "your-client-secret"
  authUrl: "https://login.microsoftonline.com/tenant/oauth2/v2.0/authorize"
  tokenUrl: "https://login.microsoftonline.com/tenant/oauth2/v2.0/token"
  scopes:
    - "openid"
    - "profile"
    - "email"
  redirectUri: "http://localhost:8080/oauth/callback"
```

### OAuth Flow

#### Authorization Code Flow

1. **Authorization Request**
   ```
   GET /oauth/authorize?response_type=code&client_id=...&redirect_uri=...&scope=...
   ```

2. **User Authentication**: User authenticates with identity provider

3. **Authorization Code**: Provider redirects with authorization code

4. **Token Exchange**
   ```bash
   curl -X POST https://provider.com/oauth/token \
     -d 'grant_type=authorization_code&code=...&redirect_uri=...'
   ```

5. **Access Token**: Receive access token for API calls

### Client Credentials Flow

For server-to-server authentication:

```bash
curl -X POST https://provider.com/oauth/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d 'grant_type=client_credentials&client_id=...&client_secret=...&scope=...'
```

### Token Refresh

```bash
curl -X POST https://provider.com/oauth/token \
  -d 'grant_type=refresh_token&refresh_token=...' \
  -d 'client_id=...' \
  -d 'client_secret=...'
```

## Development Mode Authentication

Simplified authentication for development and testing environments.

### Configuration

```bash
# Enable development mode
AUTH_MODE=development

# Optional: Set development user
DEV_USER=developer
```

### Behavior

- **No Authentication Required**: All requests are automatically authenticated
- **Default User**: Requests are attributed to "dev" or "developer" user
- **No Security**: Not suitable for production environments

### Usage

```bash
# No authentication headers needed
curl http://localhost:8080/mcp/tools

# All requests are automatically authenticated as "dev" user
```

## Adapter-Specific Authentication

Each MCP adapter can have its own authentication configuration.

### Adapter Configuration

```yaml
adapters:
  weather-api:
    endpoint: "http://weather-server:8080"
    auth:
      mode: "bearer"
      required: true
      bearerConfig:
        autoGenerate: true
        expiresIn: 24

  database-api:
    endpoint: "http://db-server:8080"
    auth:
      mode: "oauth"
      required: true
      oauthConfig:
        provider: "azure"
        clientId: "db-client-id"
        scopes: ["Database.Read", "Database.Write"]
```

### Authentication Override

Adapters can override the global authentication settings:

```yaml
# Global auth: OAuth required
auth:
  mode: "oauth"
  required: true

# Adapter-specific: No auth required
adapters:
  public-api:
    auth:
      required: false
```

## Token Management

### Token Storage

Tokens are securely managed with the following features:

- **Encrypted Storage**: Tokens encrypted at rest
- **Automatic Rotation**: Configurable token expiration and renewal
- **Revocation Support**: Ability to revoke compromised tokens
- **Audit Logging**: All token operations are logged

### Token Lifecycle

```mermaid
graph TD
    A[Token Request] --> B[Validation]
    B --> C[Generation]
    C --> D[Storage]
    D --> E[Usage]
    E --> F[Expiration Check]
    F --> G{Expired?}
    G -->|Yes| H[Renewal]
    G -->|No| E
    H --> I[Revocation Check]
    I --> J{Revoked?}
    J -->|Yes| K[Deny Access]
    J -->|No| D
```

### Token Validation

#### JWT Validation

```go
// Validate JWT token
func validateJWT(tokenString string) (*Claims, error) {
    token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
        return publicKey, nil
    })

    if err != nil {
        return nil, err
    }

    if claims, ok := token.Claims.(*Claims); ok && token.Valid {
        return claims, nil
    }

    return nil, fmt.Errorf("invalid token")
}
```

#### OAuth Token Validation

```go
// Validate OAuth token with provider
func validateOAuthToken(token string, provider string) error {
    // Validate with OAuth provider's introspection endpoint
    resp, err := http.PostForm(provider+"/introspect", url.Values{
        "token": {token},
        "client_id": {clientID},
        "client_secret": {clientSecret},
    })

    if err != nil {
        return err
    }

    var result map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&result)

    if active, ok := result["active"].(bool); !ok || !active {
        return fmt.Errorf("token not active")
    }

    return nil
}
```

## Security Best Practices

### Token Security

- **Short Expiration**: Use short-lived tokens (1-24 hours)
- **Secure Storage**: Never store tokens in client-side storage
- **HTTPS Only**: Always use HTTPS for token transmission
- **Token Rotation**: Regularly rotate tokens and refresh them

### OAuth Security

- **PKCE**: Use Proof Key for Code Exchange for public clients
- **State Parameter**: Protect against CSRF attacks
- **Scope Limitation**: Request minimal required scopes
- **Token Storage**: Use secure HTTP-only cookies for web applications

### General Security

- **Rate Limiting**: Implement rate limiting on authentication endpoints
- **Audit Logging**: Log all authentication events
- **Monitoring**: Monitor failed authentication attempts
- **Certificate Validation**: Validate SSL certificates in production

## Configuration Examples

### Complete OAuth Configuration

```yaml
# values.yaml for Helm
auth:
  mode: "oauth"
  required: true
  oauth:
    provider: "azure"
    clientId: "12345678-1234-1234-1234-123456789012"
    clientSecret: "your-client-secret"
    tenantId: "your-tenant-id"
    authUrl: "https://login.microsoftonline.com/{tenant}/oauth2/v2.0/authorize"
    tokenUrl: "https://login.microsoftonline.com/{tenant}/oauth2/v2.0/token"
    scopes:
      - "openid"
      - "profile"
      - "api://suse-ai-up/.default"
    redirectUri: "https://your-domain.com/oauth/callback"
    extraParams:
      prompt: "consent"
      domain_hint: "yourdomain.com"
```

### Bearer Token Configuration

```yaml
auth:
  mode: "bearer"
  required: true
  bearer:
    autoGenerate: true
    expiresIn: 8  # 8 hours
    issuer: "suse-ai-up"
    audience: "proxy-service"
```

### Multi-Adapter Configuration

```yaml
adapters:
  weather-api:
    endpoint: "http://weather:8080"
    auth:
      mode: "bearer"
      required: true

  database-api:
    endpoint: "http://database:8080"
    auth:
      mode: "oauth"
      required: true
      oauth:
        provider: "okta"
        clientId: "db-client-id"

  public-api:
    endpoint: "http://public:8080"
    auth:
      required: false  # No authentication needed
```

## Troubleshooting

### Common Authentication Issues

**Invalid Token Errors**
```bash
# Check token format
curl -H "Authorization: Bearer invalid-token" http://localhost:8080/health
# Response: {"error": "Invalid token"}

# Validate token structure
jwt decode your-token
```

**OAuth Configuration Issues**
```bash
# Test OAuth endpoints
curl https://your-provider.com/.well-known/openid_configuration

# Check client configuration
curl -u "client_id:client_secret" https://your-provider.com/oauth/token \
  -d 'grant_type=client_credentials'
```

**Token Expiration**
```bash
# Check token expiration
jwt decode your-token | jq .exp

# Refresh expired tokens
curl -X POST https://your-provider.com/oauth/token \
  -d 'grant_type=refresh_token&refresh_token=...'
```

### Debug Mode

Enable detailed authentication logging:

```bash
export AUTH_DEBUG=true
export LOG_LEVEL=debug
./suse-ai-up proxy
```

### Authentication Logs

```json
{
  "timestamp": "2025-12-04T12:00:00Z",
  "level": "info",
  "service": "proxy",
  "event": "auth_success",
  "user": "user@example.com",
  "method": "bearer",
  "adapter": "weather-api",
  "ip": "192.168.1.100"
}
```

## Migration Guide

### From No Authentication

**Before:**
```yaml
auth:
  enabled: false
```

**After:**
```yaml
auth:
  mode: "bearer"
  required: true
  bearer:
    autoGenerate: true
```

### From Basic Auth to OAuth

**Migration Steps:**

1. **Configure OAuth Provider**: Set up OAuth application
2. **Update Client Code**: Change authentication method
3. **Test Integration**: Verify OAuth flow works
4. **Gradual Rollout**: Migrate clients incrementally
5. **Remove Old Auth**: Disable basic auth after migration

### Token Migration

```bash
# Export existing tokens
curl http://localhost:8911/api/v1/auth/tokens > tokens_backup.json

# Import to new system
curl -X POST http://new-system:8911/api/v1/auth/tokens/import \
  -d @tokens_backup.json
```

## API Reference

### Authentication Endpoints

#### Generate Token
```http
POST /api/v1/auth/token
Content-Type: application/json

{
  "adapter": "adapter-name",
  "audience": "target-url",
  "expiresIn": 24
}
```

#### Validate Token
```http
POST /api/v1/auth/validate
Authorization: Bearer <token>

{
  "token": "jwt-token-string"
}
```

#### Revoke Token
```http
POST /api/v1/auth/revoke
Authorization: Bearer <admin-token>

{
  "tokenId": "token-to-revoke"
}
```

#### List Tokens
```http
GET /api/v1/auth/tokens
Authorization: Bearer <admin-token>
```

### OAuth Endpoints

#### Authorization
```http
GET /oauth/authorize?response_type=code&client_id=...&redirect_uri=...&scope=...
```

#### Token Exchange
```http
POST /oauth/token
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code&code=...&redirect_uri=...&client_id=...&client_secret=...
```

#### Token Introspection
```http
POST /oauth/introspect
Content-Type: application/x-www-form-urlencoded

token=...&client_id=...&client_secret=...
```

This comprehensive authentication system provides flexible, secure access control for the SUSE AI Universal Proxy while maintaining compatibility with enterprise authentication requirements.</content>
<parameter name="filePath">docs/authentication.md