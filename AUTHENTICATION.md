# SUSE AI Uniproxy Authentication Guide

## Overview

This document explains how to configure and use authentication in SUSE AI Uniproxy, supporting multiple providers including local authentication, GitHub OAuth, and Rancher OIDC.

## Quick Start

### Basic Local Authentication

```bash
# Set environment variables
export AUTH_MODE=local
export ADMIN_PASSWORD=your_secure_password

# Start the service
go run ./cmd/uniproxy
```

### GitHub OAuth Setup

```bash
export AUTH_MODE=github
export GITHUB_CLIENT_ID=your_github_app_id
export GITHUB_CLIENT_SECRET=your_github_app_secret
export GITHUB_REDIRECT_URI=http://localhost:8911/auth/github/callback
```

### Rancher OIDC Setup

```bash
export AUTH_MODE=rancher
export RANCHER_ISSUER_URL=https://your-rancher-url/oidc
export RANCHER_CLIENT_ID=your_rancher_client_id
export RANCHER_CLIENT_SECRET=your_rancher_client_secret
export RANCHER_REDIRECT_URI=http://localhost:8911/auth/rancher/callback
```

## Authentication Methods

### 1. Local Authentication

- Default password-based authentication
- Admin user created automatically with default password "admin"
- Requires password change on first login

### 2. GitHub OAuth

- OAuth 2.0 integration with GitHub
- Automatic user provisioning on first login
- Group mapping from GitHub organizations/teams

### 3. Rancher OIDC

- OIDC integration with Rancher
- Administrative users mapped from Rancher groups
- Seamless integration with Rancher user management

### 4. Development Mode

- Bypass authentication for development
- Set `DEV_MODE=true` environment variable
- Use X-User-ID headers directly

## Configuration Options

### Environment Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `AUTH_MODE` | Authentication mode: `local`, `github`, `rancher`, `dev` | `development` | No |
| `DEV_MODE` | Enable development mode (bypass auth) | `false` | No |
| `ADMIN_PASSWORD` | Initial admin password | `admin` | No |
| `FORCE_PASSWORD_CHANGE` | Force password change on first login | `true` | No |
| `PASSWORD_MIN_LENGTH` | Minimum password length | `8` | No |

#### GitHub OAuth Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `GITHUB_CLIENT_ID` | GitHub OAuth App Client ID | Yes |
| `GITHUB_CLIENT_SECRET` | GitHub OAuth App Client Secret | Yes |
| `GITHUB_REDIRECT_URI` | OAuth callback URL | Yes |
| `GITHUB_ALLOWED_ORGS` | Comma-separated list of allowed GitHub orgs | No |
| `GITHUB_ADMIN_TEAMS` | Comma-separated list of admin teams | No |

#### Rancher OIDC Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `RANCHER_ISSUER_URL` | Rancher OIDC issuer URL | Yes |
| `RANCHER_CLIENT_ID` | Rancher OIDC Client ID | Yes |
| `RANCHER_CLIENT_SECRET` | Rancher OIDC Client Secret | Yes |
| `RANCHER_REDIRECT_URI` | OIDC callback URL | Yes |
| `RANCHER_ADMIN_GROUPS` | Comma-separated list of admin groups | No |
| `RANCHER_FALLBACK_LOCAL` | Allow local auth fallback | `true` | No |

## API Usage Examples

### Local Authentication

#### Login

```bash
curl -X POST http://localhost:8911/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "admin",
    "password": "your_password"
  }'
```

Response:
```json
{
  "token": {
    "token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
    "token_type": "Bearer",
    "expires_at": "2024-01-15T10:00:00Z",
    "user_id": "admin"
  },
  "user": {
    "id": "admin",
    "name": "System Administrator",
    "email": "admin@suse.ai",
    "groups": ["mcp-admins"]
  }
}
```

#### Change Password

```bash
curl -X PUT http://localhost:8911/auth/password \
  -H "Authorization: Bearer <your_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "current_password": "old_password",
    "new_password": "new_secure_password"
  }'
```

### GitHub OAuth

#### Initiate OAuth Flow

```bash
curl -X POST http://localhost:8911/auth/oauth/login \
  -H "Content-Type: application/json" \
  -d '{
    "provider": "github"
  }'
```

Response:
```json
{
  "auth_url": "https://github.com/login/oauth/authorize?client_id=...&redirect_uri=..."
}
```

#### OAuth Callback

The user will be redirected to the auth URL, authenticate with GitHub, and GitHub will redirect back to `/auth/oauth/callback` with a code. The callback endpoint will exchange the code for a token and return user authentication.

```bash
curl -X POST http://localhost:8911/auth/oauth/callback \
  -H "Content-Type: application/json" \
  -d '{
    "code": "github_oauth_code",
    "state": "optional_state"
  }'
```

### Rancher OIDC

#### Initiate OIDC Flow

```bash
curl -X POST http://localhost:8911/auth/oauth/login \
  -H "Content-Type: application/json" \
  -d '{
    "provider": "rancher"
  }'
```

#### OIDC Callback

Similar to GitHub, the user authenticates with Rancher and gets redirected back with a code.

### Using Authenticated APIs

All protected APIs require a Bearer token:

```bash
curl -X GET http://localhost:8911/api/v1/users \
  -H "Authorization: Bearer <your_token>"
```

### User Management

#### Create User (Admin Only)

```bash
curl -X POST http://localhost:8911/api/v1/users \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "newuser",
    "name": "New User",
    "email": "user@example.com",
    "groups": ["mcp-users"],
    "auth_provider": "local"
  }'
```

#### List Users (Admin Only)

```bash
curl -X GET http://localhost:8911/api/v1/users \
  -H "Authorization: Bearer <admin_token>"
```

## User Groups and Permissions

### Default Groups

- `mcp-users`: Basic users with access to MCP services
- `mcp-admins`: Administrators with full access

### Permission Mapping

- GitHub: Users in admin teams get `mcp-admins` group
- Rancher: Users in admin groups get `mcp-admins` group
- Local: Groups assigned during user creation

## Development Mode

When `DEV_MODE=true`, authentication is bypassed and you can use X-User-ID headers:

```bash
curl -X GET http://localhost:8911/api/v1/users \
  -H "X-User-ID: admin"
```

This is useful for development and testing without setting up full authentication.

## Troubleshooting

### Common Issues

1. **403 Forbidden**: Check user permissions and group memberships
2. **Invalid Token**: Verify JWT hasn't expired, check issuer/audience
3. **OAuth Callback Errors**: Ensure redirect URIs match provider settings
4. **Rancher Group Mapping**: Confirm OIDC claims include group information

### Debug Mode

Set `LOG_LEVEL=debug` to see detailed authentication logs.

### Token Expiration

JWT tokens expire after 24 hours. Use the refresh token flow or re-authenticate.

### Password Requirements

- Minimum 8 characters (configurable)
- Admin password must be changed on first login
- Strong password recommended for production

## Security Considerations

1. **HTTPS Required**: Always use HTTPS in production
2. **Secure Secrets**: Store client secrets securely, not in environment variables
3. **Token Storage**: Store JWT tokens securely on client side
4. **Regular Rotation**: Rotate OAuth client secrets regularly
5. **Audit Logging**: Monitor authentication attempts and failures

## Migration Guide

### From No Authentication

1. Set `AUTH_MODE=local`
2. Start service (creates admin user)
3. Login as admin and change password
4. Create additional users via API

### Adding OAuth Providers

1. Configure provider settings
2. Test OAuth flow with test user
3. Update existing users if needed
4. Set as primary auth mode

### Rancher Integration

1. Configure Rancher OIDC client
2. Set admin group mappings
3. Test admin access via Rancher
4. Optionally disable local auth fallback

## API Reference

### Authentication Endpoints

- `POST /auth/login` - Local user login
- `POST /auth/oauth/login` - Initiate OAuth/OIDC flow
- `POST /auth/oauth/callback` - Handle OAuth/OIDC callback
- `PUT /auth/password` - Change password
- `POST /auth/logout` - Logout

### Protected Endpoints

All `/api/v1/users` and `/api/v1/groups` endpoints require authentication.

### Error Codes

- `401 Unauthorized`: Missing or invalid authentication
- `403 Forbidden`: Insufficient permissions
- `400 Bad Request`: Invalid request parameters</content>
<parameter name="filePath">AUTHENTICATION.md