# MCP Examples

This directory contains example MCP (Model Context Protocol) servers and usage examples for the SUSE AI Universal Proxy.

## Overview

This document demonstrates four different MCP server scenarios through the SUSE AI Universal Proxy:

1. **Authenticated Sequential Thinking MCP**: Secure sequential thinking server with Bearer token authentication
2. **HTTP MCP with Vulnerability Scoring**: Scan for MCP servers and assess security vulnerabilities
3. **HTTP MCP with Token Authentication**: Standard Bearer token authentication
4. **MCP with OAuth 2.1 Flow**: Complete OAuth 2.1 authorization flow

## Prerequisites

All examples require:
- Python 3.8+
- The SUSE AI Universal Proxy running

```bash
# Install dependencies
pip install -r requirements.txt

# Start the proxy
cd suse-ai-up-proxy && go run cmd/service/main.go
```

## Scenario 1: Authenticated Sequential Thinking MCP Server

**Goal**: Demonstrate spawning and using a secure sequential thinking MCP server through the proxy with authentication.

**Key Features**:
- Uses MCP client configuration format (`mcpClientConfig`)
- Demonstrates Bearer token authentication for LocalStdio adapters
- Shows proper 401/403 error handling for unauthorized requests
- Illustrates secure MCP server deployment through the proxy

### Setup

```bash
# 1. Start the proxy service
cd suse-ai-up-proxy && go run cmd/service/main.go

# 2. Create authenticated sequential thinking adapter
curl -X POST http://localhost:8911/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "sequential-thinking",
    "connectionType": "LocalStdio",
    "mcpClientConfig": {
      "mcpServers": {
        "sequential-thinking": {
          "command": "npx",
          "args": ["-y", "@modelcontextprotocol/server-sequential-thinking"]
        }
      }
    },
    "authentication": {
      "required": true,
      "type": "bearer",
      "token": "my-secure-token-123"
    },
    "description": "Authenticated sequential thinking MCP server"
  }' \
  | jq

# 3. Test without authentication (should fail with 401)
curl -X POST http://localhost:8911/adapters/sequential-thinking/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test-client", "version": "1.0"}}}' \
  | jq

# 4. Test with authentication (should succeed)
curl -X POST http://localhost:8911/adapters/sequential-thinking/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer my-secure-token-123" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test-client", "version": "1.0"}}}' \
  | jq

# 5. List available tools (with authentication)
curl -X POST http://localhost:8911/adapters/sequential-thinking/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer my-secure-token-123" \
  -d '{"jsonrpc": "2.0", "id": 2, "method": "tools/list"}' \
  | jq

# 6. Call sequential thinking tool (with authentication)
curl -X POST http://localhost:8911/adapters/sequential-thinking/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer my-secure-token-123" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "sequential-thinking-tool",
      "arguments": {
        "input": "analyze this problem step by step"
      }
    }
  }' \
  | jq

# 7. Clean up
curl -X DELETE http://localhost:8911/adapters/sequential-thinking
```

## Scenario 1b: Authenticated Filesystem MCP Server

**Goal**: Demonstrate secure file system access through an authenticated MCP server with directory restrictions.

**Key Features**:
- Directory-restricted file operations
- Read, write, list, and search file capabilities
- Bearer token authentication for security
- Demonstrates MCP client config with arguments

### Setup

```bash
# 1. Start the proxy service
cd suse-ai-up-proxy && go run cmd/service/main.go

# 2. Create authenticated filesystem adapter (restrict to current directory)
curl -X POST http://localhost:8911/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "filesystem",
    "connectionType": "LocalStdio",
    "mcpClientConfig": {
      "mcpServers": {
        "filesystem": {
          "command": "npx",
          "args": ["-y", "@modelcontextprotocol/server-filesystem", "."]
        }
      }
    },
    "authentication": {
      "required": true,
      "type": "bearer",
      "token": "filesystem-token-456"
    },
    "description": "Authenticated filesystem MCP server"
  }' \
  | jq

# 3. Test without authentication (should fail)
curl -X POST http://localhost:8911/adapters/filesystem/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test-client", "version": "1.0"}}}' \
  | jq

# 4. Initialize with authentication
curl -X POST http://localhost:8911/adapters/filesystem/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer filesystem-token-456" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test-client", "version": "1.0"}}}' \
  | jq

# 5. List available tools
curl -X POST http://localhost:8911/adapters/filesystem/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer filesystem-token-456" \
  -H "mcp-session-id: fs-session-123" \
  -d '{"jsonrpc": "2.0", "id": 2, "method": "tools/list"}' \
  | jq

# 6. List directory contents
curl -X POST http://localhost:8911/adapters/filesystem/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer filesystem-token-456" \
  -H "mcp-session-id: fs-session-123" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "list_directory",
      "arguments": {
        "path": "."
      }
    }
  }' \
  | jq

# 7. Clean up
curl -X DELETE http://localhost:8911/adapters/filesystem
```

## Scenario 1c: Authenticated SQLite Database MCP Server

**Goal**: Demonstrate secure database access through an authenticated MCP server with query capabilities.

**Key Features**:
- SQLite database operations (query, insert, update, delete)
- Bearer token authentication for security
- Parameterized queries to prevent SQL injection
- Read-only and read-write modes

### Setup

```bash
# 1. Start the proxy service
cd suse-ai-up-proxy && go run cmd/service/main.go

# 2. Create authenticated SQLite database adapter
curl -X POST http://localhost:8911/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "sqlite-db",
    "connectionType": "LocalStdio",
    "mcpClientConfig": {
      "mcpServers": {
        "sqlite-db": {
          "command": "npx",
          "args": ["-y", "@modelcontextprotocol/server-sqlite", "--db-path", "/tmp/test.db"]
        }
      }
    },
    "authentication": {
      "required": true,
      "type": "bearer",
      "token": "sqlite-token-101"
    },
    "description": "Authenticated SQLite database MCP server"
  }' \
  | jq

# 3. Test without authentication (should fail)
curl -X POST http://localhost:8911/adapters/sqlite-db/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test-client", "version": "1.0"}}}'

# 4. Initialize with authentication
curl -X POST http://localhost:8911/adapters/sqlite-db/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sqlite-token-101" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test-client", "version": "1.0"}}}' \
  | jq

# 5. List available tools
curl -X POST http://localhost:8911/adapters/sqlite-db/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sqlite-token-101" \
  -H "mcp-session-id: db-session-123" \
  -d '{"jsonrpc": "2.0", "id": 2, "method": "tools/list"}' \
  | jq

# 6. Create a test table
curl -X POST http://localhost:8911/adapters/sqlite-db/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sqlite-token-101" \
  -H "mcp-session-id: db-session-123" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "query",
      "arguments": {
        "sql": "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT)"
      }
    }
  }' \
  | jq

# 7. Insert data
curl -X POST http://localhost:8911/adapters/sqlite-db/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sqlite-token-101" \
  -H "mcp-session-id: db-session-123" \
  -d '{
    "jsonrpc": "2.0",
    "id": 4,
    "method": "tools/call",
    "params": {
      "name": "query",
      "arguments": {
        "sql": "INSERT INTO users (name, email) VALUES (?, ?)",
        "params": ["Alice", "alice@example.com"]
      }
    }
  }' \
  | jq

# 8. Query data
curl -X POST http://localhost:8911/adapters/sqlite-db/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sqlite-token-101" \
  -H "mcp-session-id: db-session-123" \
  -d '{
    "jsonrpc": "2.0",
    "id": 5,
    "method": "tools/call",
    "params": {
      "name": "query",
      "arguments": {
        "sql": "SELECT * FROM users"
      }
    }
  }' \
  | jq

# 9. Clean up
curl -X DELETE http://localhost:8911/adapters/sqlite-db
```

## Scenario 1e: Authenticated GitHub API MCP Server

**Goal**: Demonstrate secure API client access through an authenticated MCP server for GitHub operations.

**Key Features**:
- GitHub API integration (repositories, issues, pull requests)
- Bearer token authentication
- GitHub Personal Access Token for API access
- Rate limiting and error handling
- Read and write operations

**Important Notes**:
- Replace `github-token-202` with your actual GitHub Personal Access Token
- Ensure the token has appropriate scopes for the operations you need
- Store tokens securely and avoid hardcoding in production
- GitHub tokens should have `repo`, `issues`, and `pull_requests` scopes for full functionality

### Setup

```bash
# 1. Start the proxy service
cd suse-ai-up-proxy && go run cmd/service/main.go

# 2. Create authenticated GitHub API adapter
curl -X POST http://localhost:8911/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "github-api",
    "connectionType": "LocalStdio",
    "mcpClientConfig": {
      "mcpServers": {
        "github-api": {
          "command": "npx",
          "args": [
            "-y",
            "@modelcontextprotocol/server-github"
          ],
          "env": {
            "GITHUB_PERSONAL_ACCESS_TOKEN": "github-token-202"
          }
        }
      }
    },
    "authentication": {
      "required": true,
      "type": "bearer",
      "token": "github-token-202"
    },
    "description": "Authenticated GitHub API MCP server"
  }' \
  | jq

# 3. Test without authentication (should fail)
curl -X POST http://localhost:8911/adapters/github-api/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test-client", "version": "1.0"}}}' \
  | jq

# 4. Initialize with authentication
curl -X POST http://localhost:8911/adapters/github-api/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer github-token-202" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test-client", "version": "1.0"}}}' \
  | jq

# 5. List available tools
curl -X POST http://localhost:8911/adapters/github-api/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer github-token-202" \
  -H "mcp-session-id: github-session-123" \
  -d '{"jsonrpc": "2.0", "id": 2, "method": "tools/list"}' \
  | jq

# 6. Search repositories
curl -X POST http://localhost:8911/adapters/github-api/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer github-token-202" \
  -H "mcp-session-id: github-session-123" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "search_repositories",
      "arguments": {
        "query": "language:go stars:>1000",
        "sort": "stars",
        "order": "desc"
      }
    }
  }' \
  | jq

# 7. Get repository information
curl -X POST http://localhost:8911/adapters/github-api/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer github-token-202" \
  -H "mcp-session-id: github-session-123" \
  -d '{
    "jsonrpc": "2.0",
    "id": 4,
    "method": "tools/call",
    "params": {
      "name": "get_repository",
      "arguments": {
        "owner": "octocat",
        "repo": "Hello-World"
      }
    }
  }' \
  | jq

# 8. Clean up
curl -X DELETE http://localhost:8911/adapters/github-api
```

**Goal**: Demonstrate running custom Python MCP servers with authentication through the proxy.

**Key Features**:
- Execute custom Python MCP scripts
- Bearer token authentication
- Full MCP protocol support
- Custom tool definitions

### Setup

First, create a custom MCP server script (`custom_mcp_server.py`):

```python
#!/usr/bin/env python3
import asyncio
import sys
from mcp import Tool
from mcp.server import Server
from mcp.types import TextContent, PromptMessage
import mcp.server.stdio

server = Server("custom-mcp-server")

@server.tool()
async def custom_greeting(name: str = "World") -> str:
    """Generate a personalized greeting."""
    return f"Hello, {name}! Welcome to the authenticated MCP server."

@server.tool()
async def calculate_area(length: float, width: float) -> float:
    """Calculate the area of a rectangle."""
    return length * width

async def main():
    async with mcp.server.stdio.stdio_server() as (read_stream, write_stream):
        await server.run(
            read_stream,
            write_stream,
            server.create_initialization_options()
        )

if __name__ == "__main__":
    asyncio.run(main())
```

Then run the authenticated adapter:

```bash
# 1. Start the proxy service
cd suse-ai-up-proxy && go run cmd/service/main.go

# 2. Create authenticated custom Python adapter
curl -X POST http://localhost:8911/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "custom-python",
    "connectionType": "LocalStdio",
    "mcpClientConfig": {
      "mcpServers": {
        "custom-python": {
          "command": "python3",
          "args": ["/path/to/custom_mcp_server.py"],
          "env": {
            "PYTHONPATH": "/path/to/server",
            "CUSTOM_API_KEY": "custom-python-token-789"
          }
        }
      }
    },
    "authentication": {
      "required": true,
      "type": "bearer",
      "token": "custom-python-token-789"
    },
    "description": "Authenticated custom Python MCP server"
  }'

# 3. Test without authentication (should fail)
curl -X POST http://localhost:8911/adapters/custom-python/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test-client", "version": "1.0"}}}'

# 4. Initialize with authentication
curl -X POST http://localhost:8911/adapters/custom-python/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer custom-python-token-789" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test-client", "version": "1.0"}}}'

# 5. List available tools
curl -X POST http://localhost:8911/adapters/custom-python/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer custom-python-token-789" \
  -H "mcp-session-id: custom-session-123" \
  -d '{"jsonrpc": "2.0", "id": 2, "method": "tools/list"}'

# 6. Call custom greeting tool
curl -X POST http://localhost:8911/adapters/custom-python/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer custom-python-token-789" \
  -H "mcp-session-id: custom-session-123" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "custom_greeting",
      "arguments": {
        "name": "Alice"
      }
    }
  }'

# 7. Clean up
curl -X DELETE http://localhost:8911/adapters/custom-python
```

## Scenario 2: HTTP MCP with Vulnerability Scoring

**Goal**: Scan for MCP servers and assess security vulnerabilities with automatic scoring.

**Vulnerability Levels**:
- `"high"`: No authentication required
- `"medium"`: Token-based authentication
- `"low"`: OAuth 2.0/OAuth 2.1 compliant

### Setup

```bash
# Start unauthenticated MCP server (high vulnerability)
MCP_TRANSPORT=http python src/main_no_auth.py

# Scan for MCP servers
curl -X POST http://localhost:8911/scan \
  -H "Content-Type: application/json" \
  -d '{
    "scanRanges": ["127.0.0.1/32"],
    "ports": [8002]
  }' \
  | jq

# Check discovered servers with vulnerability scores
curl http://localhost:8911/servers

# Expected response includes vulnerability_score field
{
  "id": "mcp-...",
  "address": "http://127.0.0.1:8002",
  "protocol": "MCP",
  "connection": "StreamableHttp",
  "status": "healthy",
  "vulnerability_score": "high",
  "metadata": {
    "detectionMethod": "streamable-http",
    "auth_type": "none"
  }
}

# Create adapter for discovered server
curl -X POST http://localhost:8911/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "no-auth-mcp",
    "imageName": "mcp-proxy",
    "imageVersion": "1.0.0",
    "connectionType": "StreamableHttp",
    "environmentVariables": {
      "MCP_PROXY_URL": "http://localhost:8002"
    }
  }' \
  | jq

# Initialize and test tools (no auth required)
curl -X POST http://localhost:8911/adapters/no-auth-mcp/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test-client", "version": "1.0.0"}}}' \
  | jq

# List tools
curl -X POST http://localhost:8911/adapters/no-auth-mcp/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "mcp-session-id: YOUR_SESSION_ID_HERE" \
  -d '{"jsonrpc": "2.0", "id": 2, "method": "tools/list"}' \
  | jq

# Call multiply tool
curl -X POST http://localhost:8911/adapters/no-auth-mcp/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "mcp-session-id: YOUR_SESSION_ID_HERE" \
  -d '{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "multiply", "arguments": {"a": 4, "b": 7}}}' \
  | jq
```

## Scenario 3: HTTP MCP with Token Authentication

**Goal**: Demonstrate standard Bearer token authentication.

### Setup

```bash
# Start authenticated MCP server
MCP_TRANSPORT=http python src/main.py

# Create adapter
curl -X POST http://localhost:8911/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "token-auth-mcp",
    "imageName": "mcp-proxy",
    "imageVersion": "1.0.0",
    "connectionType": "StreamableHttp",
    "environmentVariables": {
      "MCP_PROXY_URL": "http://localhost:8001"
    }
  }'

# Initialize with auth token
curl -X POST http://localhost:8911/adapters/token-auth-mcp/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer mcp-example-token-12345" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test-client", "version": "1.0.0"}}}'

# List available tools (with auth)
curl -X POST http://localhost:8911/adapters/token-auth-mcp/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer mcp-example-token-12345" \
  -H "Accept: application/json, text/event-stream" \
  -H "mcp-session-id: YOUR_SESSION_ID_HERE" \
  -d '{"jsonrpc": "2.0", "id": 2, "method": "tools/list"}'

# Call get_server_info tool (with auth)
curl -X POST http://localhost:8911/adapters/token-auth-mcp/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer mcp-example-token-12345" \
  -H "Accept: application/json, text/event-stream" \
  -H "mcp-session-id: YOUR_SESSION_ID_HERE" \
  -d '{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "get_server_info"}}'
```

## Scenario 4: MCP with OAuth 2.1 Flow

**Goal**: Demonstrate complete OAuth 2.1 authorization flow with automatic token management.

### Setup

```bash
# Start OAuth-protected MCP server
MCP_TRANSPORT=http python src/oauth_server.py

# Create adapter
curl -X POST http://localhost:8911/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "oauth-mcp",
    "imageName": "mcp-proxy",
    "imageVersion": "1.0.0",
    "connectionType": "StreamableHttp",
    "environmentVariables": {
      "MCP_PROXY_URL": "http://localhost:8004"
    }
  }'

# Initiate OAuth authorization
curl -X POST http://localhost:8911/adapters/oauth-mcp/auth/authorize \
  -H "Content-Type: application/json" \
  -d '{
    "clientInfo": {"name": "mcp-client", "version": "1.0"},
    "resource": "https://mcp.example.com"
  }'



# Check authorization status
curl http://localhost:8911/adapters/oauth-mcp/auth/status

# Once authorized, make MCP requests (tokens handled automatically)
curl -X POST http://localhost:8911/adapters/oauth-mcp/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test-client", "version": "1.0.0"}}}'

# List tools (OAuth tokens handled automatically)
curl -X POST http://localhost:8911/adapters/oauth-mcp/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "mcp-session-id: YOUR_SESSION_ID_HERE" \
  -d '{"jsonrpc": "2.0", "id": 2, "method": "tools/list"}'

# Call add tool (OAuth tokens handled automatically)
curl -X POST http://localhost:8911/adapters/oauth-mcp/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "mcp-session-id: YOUR_SESSION_ID_HERE" \
  -d '{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "add", "arguments": {"a": 10, "b": 15}}}'
```

## Available MCP Servers

### Sequential Thinking MCP Server
- **Transport**: stdio via proxy
- **Authentication**: Bearer token (configurable per adapter)
- **Tools**: Sequential thinking and analysis tools
- **Command**: `npx -y @modelcontextprotocol/server-sequential-thinking`
- **Security**: Demonstrates authenticated LocalStdio adapters

### Filesystem MCP Server
- **Transport**: stdio via proxy
- **Authentication**: Bearer token (configurable per adapter)
- **Tools**: File reading, writing, listing, and search operations
- **Command**: `npx -y @modelcontextprotocol/server-filesystem /path/to/allowed/directory`
- **Security**: Directory-restricted file access with authentication

### Custom Python Script MCP Server
- **Transport**: stdio via proxy
- **Authentication**: Bearer token (configurable per adapter)
- **Tools**: Custom tools defined in Python script
- **Command**: `python3 /path/to/custom/mcp_script.py`
- **Environment Variables**: Customizable (e.g., `PYTHONPATH`, `CUSTOM_API_KEY`)
- **Security**: Execute custom MCP servers with authentication

### SQLite Database MCP Server
- **Transport**: stdio via proxy
- **Authentication**: Bearer token (configurable per adapter)
- **Tools**: Database query, insert, update, delete operations
- **Command**: `npx -y @modelcontextprotocol/server-sqlite --db-path /path/to/database.db`
- **Security**: Parameterized queries with authentication

### GitHub API MCP Server
- **Transport**: stdio via proxy
- **Authentication**: Bearer token (configurable per adapter) + GitHub Personal Access Token
- **Tools**: Repository search, issue management, pull request operations
- **Command**: `npx -y @modelcontextprotocol/server-github`
- **Environment Variables**: `GITHUB_PERSONAL_ACCESS_TOKEN` (required for GitHub API access)
- **Security**: GitHub API access with authentication and token management

### main.py
- **Transport**: stdio (default) or HTTP
- **Authentication**: Bearer token when HTTP transport
- **Tools**: `add`, `multiply`, `get_server_info`
- **Port**: 8001 (HTTP mode)

### main_no_auth.py
- **Transport**: HTTP only
- **Authentication**: None (for vulnerability testing)
- **Tools**: `add`, `multiply`, `get_server_info`
- **Port**: 8002

### oauth_server.py
- **Transport**: HTTP only
- **Authentication**: OAuth 2.1 with built-in authorization server
- **Tools**: `add`, `multiply`, `get_server_info`
- **Ports**: 8003 (OAuth server), 8004 (MCP server)

## Security Features

- **Per-Adapter Authentication**: Configure Bearer token requirements for individual LocalStdio adapters
- **Automatic Vulnerability Assessment**: Discovery service scores MCP servers based on authentication
- **Proxy Endpoint Protection**: Authenticated proxy adapters receive "low" vulnerability scores
- **OAuth 2.1 Compliance**: Full authorization code flow with PKCE
- **Token Management**: Automatic refresh and secure storage
- **Session Handling**: Per-adapter session management with activity tracking

## Development Notes

- Sequential thinking server demonstrates authenticated LocalStdio adapters
- All servers use FastMCP framework
- HTTP servers run on different ports to avoid conflicts
- OAuth server includes both authorization server and protected resource
- Proxy handles all authentication flows transparently
- LocalStdio adapters support both legacy command/args and MCP client config formats

## Expected Responses

### Tools/List Response
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "tools": [
      {
        "name": "add",
        "description": "Add two numbers",
        "inputSchema": {
          "type": "object",
          "properties": {
            "a": {"type": "integer"},
            "b": {"type": "integer"}
          },
          "required": ["a", "b"]
        }
      },
      {
        "name": "multiply",
        "description": "Multiply two numbers",
        "inputSchema": {
          "type": "object",
          "properties": {
            "a": {"type": "integer"},
            "b": {"type": "integer"}
          },
          "required": ["a", "b"]
        }
      },
      {
        "name": "get_server_info",
        "description": "Get server information",
        "inputSchema": {
          "type": "object",
          "properties": {}
        }
      }
    ]
  }
}
```

### Tool Call Responses

**Add Tool (5 + 3)**:
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "result": 8
  }
}
```

**Multiply Tool (4 × 7)**:
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "result": 28
  }
}
```

**Get Server Info**:
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "name": "MCP Example Server with Auth",
    "version": "1.0.0",
    "description": "Example MCP server",
    "auth_required": true,
    "supported_protocols": ["2024-11-05"]
  }
}
```

## Troubleshooting

**Port Conflicts**: Each scenario uses different ports (8001-8004)
**Dependencies**: Ensure all Python packages are installed
**Proxy Connection**: Verify proxy is running on port 8911
**Session IDs**: For HTTP adapters, copy from initialize response headers. For LocalStdio adapters, session IDs are automatically generated and returned in the `mcp-session-id` header
**Authentication**: Include appropriate auth headers for Scenarios 3 & 4
