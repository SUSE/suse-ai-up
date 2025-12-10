# User and Group Management

The SUSE AI Universal Proxy includes a comprehensive user and group management system that enables fine-grained access control for MCP servers and route assignments.

## Overview

The user/group management system provides:

- **User Management**: Create, update, and manage user accounts
- **Group Management**: Organize users into groups with shared permissions
- **Route Assignments**: Control which users/groups can access specific MCP servers
- **Auto-Spawning**: Automatic adapter creation based on permissions
- **Permission System**: Role-based access control with wildcard patterns

## Architecture

### Components

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│     Users       │    │     Groups      │    │ Route           │
│ - ID            │    │ - ID            │    │ Assignments     │
│ - Name          │    │ - Name          │    │ - Server ID     │
│ - Email         │    │ - Members       │    │ - User IDs      │
│ - Groups        │    │ - Permissions   │    │ - Group IDs     │
└─────────────────┘    └─────────────────┘    │ - Auto-spawn    │
                                              └─────────────────┘
```

### Permission System

Permissions use a hierarchical pattern matching system:

- `server:*` - Access to all servers
- `server:weather-*` - Access to all weather-related servers
- `server:weather-api:read` - Read-only access to weather-api server
- `server:weather-api:write` - Write access to weather-api server
- `user:manage` - Permission to manage users
- `group:manage` - Permission to manage groups

## Default Groups

The system initializes with three default groups:

### MCP Users (`mcp-users`)
- **Permissions**: `server:read`, `adapter:create`
- **Description**: Basic users with read access to MCP servers and ability to create adapters

### MCP Admins (`mcp-admins`)
- **Permissions**: `server:*`, `user:manage`, `group:manage`, `adapter:*`
- **Description**: Administrators with full access to all systems

### Weather Users (`weather-users`)
- **Permissions**: `server:weather-*`
- **Description**: Specialized group for weather API access

## API Endpoints

### User Management

#### Create User
```http
POST /api/v1/users
Content-Type: application/json

{
  "id": "user123",
  "name": "John Doe",
  "email": "john@example.com",
  "groups": ["mcp-users"]
}
```

#### List Users
```http
GET /api/v1/users
```

#### Get User
```http
GET /api/v1/users/{id}
```

#### Update User
```http
PUT /api/v1/users/{id}
Content-Type: application/json

{
  "name": "John Smith",
  "groups": ["mcp-users", "weather-users"]
}
```

#### Delete User
```http
DELETE /api/v1/users/{id}
```

### Group Management

#### Create Group
```http
POST /api/v1/groups
Content-Type: application/json

{
  "id": "weather-team",
  "name": "Weather Team",
  "description": "Team with access to weather APIs",
  "permissions": ["server:weather-*"]
}
```

#### List Groups
```http
GET /api/v1/groups
```

#### Get Group
```http
GET /api/v1/groups/{id}
```

#### Update Group
```http
PUT /api/v1/groups/{id}
Content-Type: application/json

{
  "name": "Weather Operations Team",
  "permissions": ["server:weather-*", "server:forecast:*"]
}
```

#### Delete Group
```http
DELETE /api/v1/groups/{id}
```

#### Add User to Group
```http
POST /api/v1/groups/{id}/members
Content-Type: application/json

{
  "userId": "user123"
}
```

#### Remove User from Group
```http
DELETE /api/v1/groups/{id}/members/{userId}
```

### Route Assignments

#### Create Route Assignment
```http
POST /api/v1/registry/{serverId}/routes
Content-Type: application/json

{
  "userIds": ["user123"],
  "groupIds": ["weather-users"],
  "autoSpawn": true,
  "permissions": "read"
}
```

#### List Route Assignments
```http
GET /api/v1/registry/{serverId}/routes
```

#### Update Route Assignment
```http
PUT /api/v1/registry/{serverId}/routes/{assignmentId}
Content-Type: application/json

{
  "autoSpawn": false,
  "permissions": "write"
}
```

#### Delete Route Assignment
```http
DELETE /api/v1/registry/{serverId}/routes/{assignmentId}
```

## Auto-Spawning Workflow

When a user requests access to an MCP server:

1. **Permission Check**: System verifies user has access via route assignments
2. **Adapter Lookup**: Checks if user already has an adapter for the server
3. **Auto-Spawn**: If no adapter exists and auto-spawn is enabled, creates one
4. **Access Grant**: Returns adapter endpoint for user access

### Example Flow

```bash
# User requests weather server access
curl "http://localhost:8913/api/v1/registry/weather-server/access"

# System checks permissions for user "john"
# - User is in "weather-users" group
# - Group has "server:weather-*" permission
# - Route assignment allows auto-spawn

# System creates adapter automatically
# Returns adapter endpoint
{
  "server": { /* server metadata */ },
  "adapterUrl": "http://localhost:8911/api/v1/adapters/auto-weather-server-john/mcp",
  "autoSpawned": true
}
```

## Security Considerations

### Authentication
- All API endpoints require authentication via `X-User-ID` header
- Admin operations require appropriate permissions
- JWT tokens used for adapter authentication

### Authorization
- Permission checks performed before all operations
- Group membership validated in real-time
- Route assignments enforce access control

### Audit Logging
- All user/group changes logged
- Route assignment modifications tracked
- Adapter creation/deletion audited

## Integration with MCP Registry

### Default Route Assignments

Well-known MCP servers include default route assignments:

```json
{
  "id": "filesystem",
  "routeAssignments": [{
    "id": "filesystem-default",
    "userIds": [],
    "groupIds": ["mcp-users"],
    "autoSpawn": true,
    "permissions": "read"
  }],
  "autoSpawn": {
    "enabled": true,
    "connectionType": "LocalStdio",
    "command": "npx",
    "args": ["@modelcontextprotocol/server-filesystem"]
  }
}
```

### Sync Process

During registry synchronization:
1. Servers loaded with default route assignments
2. Auto-spawn configuration applied
3. Permissions set based on server type

## Monitoring & Observability

### Metrics
- User/group creation/deletion rates
- Permission check latency
- Auto-spawn success/failure rates
- Route assignment utilization

### Logging
- Structured logs for all operations
- Permission check results
- Auto-spawn events
- Group membership changes

## Troubleshooting

### Common Issues

**Permission Denied**
```
Error: "Insufficient permissions to manage users"
Solution: Ensure user has appropriate group membership
```

**Auto-spawn Failed**
```
Error: "Failed to create adapter: server does not support auto-spawning"
Solution: Check server has AutoSpawn configuration
```

**Group Not Found**
```
Error: "group with ID X not found"
Solution: Verify group exists and user has permission to view it
```

### Debug Commands

```bash
# Check user permissions
curl -H "X-User-ID: john" http://localhost:8913/api/v1/users/john

# List user's groups
curl -H "X-User-ID: john" http://localhost:8913/api/v1/groups

# Check route assignments
curl -H "X-User-ID: john" http://localhost:8913/api/v1/registry/weather-server/routes
```

## Migration Guide

### From No User Management

1. **Initialize Default Groups**: System creates default groups automatically
2. **Add Users**: Create user accounts via API
3. **Assign Groups**: Add users to appropriate groups
4. **Configure Routes**: Set up route assignments for servers
5. **Enable Auto-spawn**: Configure servers for automatic adapter creation

### From Basic User ID

1. **Migrate Users**: Create user records for existing user IDs
2. **Assign Groups**: Add users to appropriate groups
3. **Update Permissions**: Configure route assignments
4. **Test Access**: Verify auto-spawning works correctly

This user/group management system provides the foundation for secure, scalable MCP server access with automatic adapter provisioning.</content>
<parameter name="filePath">docs/services/user-group-management.md