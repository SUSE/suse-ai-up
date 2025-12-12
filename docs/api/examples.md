# SUSE AI Uniproxy Examples

## Quick Start

### 1. Start the Service
```bash
# Start all services
./suse-ai-up all

# Or start individual services
./suse-ai-up-discovery &
./suse-ai-up-registry &
./suse-ai-up-plugins &
./suse-ai-up uniproxy
```

### 2. Check Health
```bash
curl http://localhost:8911/health
# {"status":"healthy","timestamp":"2024-01-01T12:00:00Z","version":"1.0.0"}
```

### 3. Create Your First Adapter
```bash
curl -X POST http://localhost:8911/api/v1/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "filesystem-adapter",
    "connectionType": "LocalStdio",
    "mcpClientConfig": {
      "mcpServers": {
        "filesystem": {
          "command": "npx",
          "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
        }
      }
    }
  }'
```

### 4. List Available Tools
```bash
curl http://localhost:8911/api/v1/adapters/filesystem-adapter/tools
```

## MCP Server Integration Examples

### GitHub Integration
```bash
# Create GitHub adapter
curl -X POST http://localhost:8911/api/v1/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "github-adapter",
    "connectionType": "RemoteHttp",
    "apiBaseUrl": "https://api.github.com",
    "authentication": {
      "required": true,
      "type": "bearer",
      "bearerToken": {
        "token": "ghp_your_github_token_here"
      }
    },
    "mcpClientConfig": {
      "mcpServers": {
        "github": {
          "command": "npx",
          "args": ["-y", "@modelcontextprotocol/server-github"]
        }
      }
    }
  }'

# List repositories
curl -X POST http://localhost:8911/api/v1/adapters/github-adapter/tools/list-repos/call \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ghp_your_github_token_here" \
  -d '{"owner": "octocat"}'
```

### Slack Integration
```bash
# Create Slack adapter
curl -X POST http://localhost:8911/api/v1/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "slack-adapter",
    "connectionType": "RemoteHttp",
    "apiBaseUrl": "https://slack.com/api",
    "authentication": {
      "required": true,
      "type": "bearer",
      "bearerToken": {
        "token": "xoxb-your-slack-bot-token"
      }
    },
    "mcpClientConfig": {
      "mcpServers": {
        "slack": {
          "command": "npx",
          "args": ["-y", "@modelcontextprotocol/server-slack"]
        }
      }
    }
  }'

# Send message
curl -X POST http://localhost:8911/api/v1/adapters/slack-adapter/tools/chat-postMessage/call \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer xoxb-your-slack-bot-token" \
  -d '{
    "channel": "#general",
    "text": "Hello from SUSE AI Uniproxy!"
  }'
```

### Database Integration (PostgreSQL)
```bash
# Create PostgreSQL adapter
curl -X POST http://localhost:8911/api/v1/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "postgres-adapter",
    "connectionType": "LocalStdio",
    "mcpClientConfig": {
      "mcpServers": {
        "postgres": {
          "command": "npx",
          "args": ["-y", "@modelcontextprotocol/server-postgres"],
          "env": {
            "POSTGRES_CONNECTION_STRING": "postgresql://user:password@localhost:5432/mydb"
          }
        }
      }
    }
  }'

# Query database
curl -X POST http://localhost:8911/api/v1/adapters/postgres-adapter/tools/query/call \
  -H "Content-Type: application/json" \
  -d '{
    "sql": "SELECT * FROM users LIMIT 10"
  }'
```

## VirtualMCP Examples

### Custom Tool Creation
```bash
# Create VirtualMCP adapter with custom tools
curl -X POST http://localhost:8911/api/v1/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "custom-tools",
    "connectionType": "LocalStdio",
    "apiBaseUrl": "https://api.example.com",
    "tools": [
      {
        "name": "get_weather",
        "description": "Get current weather for a location",
        "inputSchema": {
          "type": "object",
          "properties": {
            "location": {
              "type": "string",
              "description": "City name"
            }
          },
          "required": ["location"]
        }
      },
      {
        "name": "send_email",
        "description": "Send an email",
        "inputSchema": {
          "type": "object",
          "properties": {
            "to": {"type": "string"},
            "subject": {"type": "string"},
            "body": {"type": "string"}
          },
          "required": ["to", "subject", "body"]
        }
      }
    ],
    "mcpClientConfig": {
      "mcpServers": {
        "virtualmcp": {
          "command": "tsx",
          "args": ["templates/virtualmcp-server.ts"],
          "env": {
            "SERVER_NAME": "custom-tools",
            "API_BASE_URL": "https://api.example.com"
          }
        }
      }
    }
  }'

# Call custom tool
curl -X POST http://localhost:8911/api/v1/adapters/custom-tools/tools/get_weather/call \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer virtualmcp-token" \
  -d '{"location": "San Francisco"}'
```

## Registry Management

### Browse Available Servers
```bash
# List all registry servers
curl http://localhost:8911/api/v1/registry

# Get specific server details
curl http://localhost:8911/api/v1/registry/github

# Browse with filtering
curl "http://localhost:8911/api/v1/registry/browse?category=productivity"
```

### Sync Official Registry
```bash
# Sync with mcpservers.org
curl -X POST http://localhost:8911/api/v1/registry/sync/official
```

### Upload Custom Server
```bash
# Upload local MCP server
curl -X POST http://localhost:8911/api/v1/registry/upload/local-mcp \
  -F "file=@/path/to/server-package.json" \
  -F "config={\"name\": \"my-server\", \"version\": \"1.0.0\"}"
```

## Discovery Examples

### Network Scanning
```bash
# Start network scan
curl -X POST http://localhost:8911/api/v1/discovery/scan \
  -H "Content-Type: application/json" \
  -d '{
    "scanRanges": ["192.168.1.0/24"],
    "ports": ["8000", "8001", "9000"],
    "timeout": "30s",
    "maxConcurrent": 10
  }'

# Check scan status
curl http://localhost:8911/api/v1/discovery/scan/scan-12345

# List discovered servers
curl http://localhost:8911/api/v1/discovery/servers
```

### Register Discovered Server
```bash
# Register discovered server as adapter
curl -X POST http://localhost:8911/api/v1/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "discovered-server",
    "connectionType": "RemoteHttp",
    "apiBaseUrl": "http://192.168.1.100:8000",
    "source": "discovery"
  }'
```

## Plugin Management

### Register Service
```bash
# Register a plugin service
curl -X POST http://localhost:8911/api/v1/plugins/register \
  -H "Content-Type: application/json" \
  -d '{
    "serviceId": "my-plugin",
    "serviceType": "http-proxy",
    "config": {
      "targetUrl": "https://api.example.com"
    }
  }'
```

### Service Health Monitoring
```bash
# Check service health
curl http://localhost:8911/api/v1/plugins/services/my-plugin/health

# List all services
curl http://localhost:8911/api/v1/plugins/services

# List services by type
curl http://localhost:8911/api/v1/plugins/services/type/http-proxy
```

## Advanced Examples

### Multi-Server Adapter
```bash
# Create adapter with multiple MCP servers
curl -X POST http://localhost:8911/api/v1/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "multi-server-adapter",
    "connectionType": "LocalStdio",
    "mcpClientConfig": {
      "mcpServers": {
        "filesystem": {
          "command": "npx",
          "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
        },
        "git": {
          "command": "npx",
          "args": ["-y", "@modelcontextprotocol/server-git", "--repository", "/path/to/repo"]
        },
        "sqlite": {
          "command": "npx",
          "args": ["-y", "@modelcontextprotocol/server-sqlite", "--db-path", "/tmp/data.db"]
        }
      }
    }
  }'
```

### Session Management
```bash
# Create new session
curl -X POST http://localhost:8911/api/v1/adapters/my-adapter/sessions

# List sessions
curl http://localhost:8911/api/v1/adapters/my-adapter/sessions

# Get session details
curl http://localhost:8911/api/v1/adapters/my-adapter/sessions/session-123

# Delete session
curl -X DELETE http://localhost:8911/api/v1/adapters/my-adapter/sessions/session-123
```

### Resource Access
```bash
# List resources
curl http://localhost:8911/api/v1/adapters/my-adapter/resources

# Read specific resource
curl http://localhost:8911/api/v1/adapters/my-adapter/resources/file:///tmp/data.txt

# List prompts
curl http://localhost:8911/api/v1/adapters/my-adapter/prompts

# Get specific prompt
curl "http://localhost:8911/api/v1/adapters/my-adapter/prompts/code-review?language=javascript&complexity=high"
```

## Docker Deployment Examples

### Docker Compose
```yaml
version: '3.8'
services:
  suse-ai-up:
    image: suse/suse-ai-up:latest
    ports:
      - "8911:8911"
    environment:
      - AUTH_MODE=production
      - PORT=8911
    volumes:
      - ./config:/app/config
      - ./logs:/app/logs
```

### Kubernetes Deployment
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: suse-ai-up
spec:
  replicas: 1
  selector:
    matchLabels:
      app: suse-ai-up
  template:
    metadata:
      labels:
        app: suse-ai-up
    spec:
      containers:
      - name: suse-ai-up
        image: suse/suse-ai-up:latest
        ports:
        - containerPort: 8911
        env:
        - name: AUTH_MODE
          value: "production"
        - name: PORT
          value: "8911"
        volumeMounts:
        - name: config
          mountPath: /app/config
      volumes:
      - name: config
        configMap:
          name: suse-ai-up-config
```

## Troubleshooting

### Common Issues

#### Connection Refused
```bash
# Check if service is running
curl http://localhost:8911/health

# Check logs
curl http://localhost:8911/api/v1/monitoring/logs
```

#### Authentication Failed
```bash
# Validate token
curl -X POST http://localhost:8911/api/v1/adapters/my-adapter/token/validate \
  -H "Authorization: Bearer your-token"

# Check adapter configuration
curl http://localhost:8911/api/v1/adapters/my-adapter
```

#### Tool Not Found
```bash
# List available tools
curl http://localhost:8911/api/v1/adapters/my-adapter/tools

# Check adapter status
curl http://localhost:8911/api/v1/adapters/my-adapter/status
```

### Debug Commands
```bash
# Get adapter logs
curl http://localhost:8911/api/v1/adapters/my-adapter/logs

# Check cache status
curl http://localhost:8911/api/v1/monitoring/cache

# View metrics
curl http://localhost:8911/api/v1/monitoring/metrics
```