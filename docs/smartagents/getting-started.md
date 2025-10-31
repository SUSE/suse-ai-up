# SUSE AI Smart Agents API Service - Getting Started

This guide will help you get the SUSE AI Smart Agents API service up and running quickly.

## Prerequisites

### System Requirements
- Go 1.23 or later
- Ollama (for local models) or API keys for cloud providers
- API keys for AI providers (Groq, OpenAI, Anthropic, etc.)

### Required Software
- [Go 1.23+](https://golang.org/dl/) - For building and running the service
- [Ollama](https://ollama.ai/) - For local AI models (optional but recommended)
- [Git](https://git-scm.com/) - For cloning the repository

### API Keys (choose based on your needs)
- **Groq**: Get API key from [groq.com](https://groq.com)
- **OpenAI**: Get API key from [platform.openai.com](https://platform.openai.com)
- **Anthropic**: Get API key from [console.anthropic.com](https://console.anthropic.com)
- **Ollama**: No API key needed, runs locally

## Installation

### 1. Clone the Repository
```bash
git clone <repository-url>
cd rancher-sa-api/suse-ai-up-smartagents
```

### 2. Install Dependencies
```bash
go mod download
```

### 3. Set Up Ollama (Optional but Recommended)
```bash
# Install Ollama
curl -fsSL https://ollama.ai/install.sh | sh

# Start Ollama service
ollama serve

# Pull a model (in another terminal)
ollama pull llama3.2:latest
```

## Running the Server

### Basic Startup
```bash
go run cmd/main.go
```

The server starts on `http://localhost:8910` with:
- REST API endpoints
- Swagger documentation at `/swagger/`
- Health check at `/health`

### With Environment Variables
```bash
# Set API keys
export GROQ_API_KEY="gsk_your_key_here"
export OPENAI_API_KEY="sk-your_key_here"
export ANTHROPIC_API_KEY="sk-ant-your_key_here"

# Start server
go run cmd/main.go
```

### Configuration Options
```bash
# Custom port
PORT=8080 go run cmd/main.go

# Enable debug logging
LOG_LEVEL=debug go run cmd/main.go

# Database path (SQLite)
DATABASE_PATH=./data/smartagents.db go run cmd/main.go
```

## Creating Your First Agent

### 1. Configure an Agent
Use the API to create your first agent:

```bash
curl -X POST http://localhost:8910/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-first-agent",
    "supervisor": {
      "provider": "groq",
      "api": "gsk_your_groq_key_here",
      "model": "llama-3.3-70b-versatile"
    },
    "worker": {
      "provider": "ollama",
      "url": "http://localhost:11434",
      "model": "llama3.2:latest"
    }
  }'
```

**Response:**
```json
{
  "id": "agent_123",
  "name": "my-first-agent",
  "supervisor": {
    "provider": "groq",
    "model": "llama-3.3-70b-versatile"
  },
  "worker": {
    "provider": "ollama",
    "model": "llama3.2:latest"
  },
  "created_at": "2025-10-28T12:00:00Z",
  "status": "active"
}
```

### 2. Test the Agent
Send a test message to verify everything works:

```bash
curl -X POST http://localhost:8910/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "my-first-agent",
    "messages": [
      {"role": "user", "content": "Hello! Can you tell me about yourself?"}
    ],
    "stream": true
  }'
```

### 3. Check Available Agents
List all configured agents:

```bash
curl http://localhost:8910/v1/models
```

**Response:**
```json
{
  "object": "list",
  "data": [
    {
      "id": "my-first-agent",
      "object": "model",
      "created": 1640995200,
      "owned_by": "smartagents"
    }
  ]
}
```

## Using the Example Chat Client

### 1. Navigate to Examples
```bash
cd ../examples/smartagents
```

### 2. Run the Chat Client
```bash
go run main.go my-first-agent http://localhost:8910
```

### 3. Start Chatting
```
🤖 Rancher SA Chat Client
Agent: my-first-agent
API: http://localhost:8910
Type 'quit' or 'exit' to end the conversation
=================================================

👤 You: Hello, how are you?

🤖 Assistant: Hello! I'm doing well, thank you for asking. How can I help you today?

⏱️  Request completed in 2.34s
📊 Tokens used: 247 (Supervisor: 89, Worker: 158)
```

## Advanced Configuration

### Multiple Agents
Create different agents for different purposes:

```bash
# Code review agent
curl -X POST http://localhost:8910/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "code-reviewer",
    "supervisor": {
      "provider": "openai",
      "api": "sk-your_openai_key",
      "model": "gpt-4"
    },
    "worker": {
      "provider": "ollama",
      "model": "codellama:7b"
    }
  }'

# Creative writing agent
curl -X POST http://localhost:8910/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "creative-writer",
    "supervisor": {
      "provider": "anthropic",
      "api": "sk-ant-your_anthropic_key",
      "model": "claude-3-haiku-20240307"
    },
    "worker": {
      "provider": "ollama",
      "model": "llama2:7b"
    }
  }'
```

### Environment Variables Configuration
Create a `.env` file for persistent configuration:

```bash
# .env file
GROQ_API_KEY=gsk_your_key_here
OPENAI_API_KEY=sk-your_key_here
ANTHROPIC_API_KEY=sk-ant-your_key_here
PORT=8910
LOG_LEVEL=info
DATABASE_PATH=./data/smartagents.db
```

Load environment variables and start:
```bash
source .env
go run cmd/main.go
```

## MCP Registry Integration

### Upload MCP Servers
```bash
# Upload custom MCP server configurations
curl -X POST http://localhost:8910/registry/upload/bulk \
  -H "Content-Type: application/json" \
  -d '[
    {
      "name": "filesystem-server",
      "description": "File system access capabilities",
      "packages": [{
        "registryType": "npm",
        "identifier": "@modelcontextprotocol/server-filesystem",
        "transport": {"type": "stdio"}
      }]
    }
  ]'
```

### Browse Available Servers
```bash
# Search MCP servers
curl "http://localhost:8910/registry/browse?transport=stdio"
```

## Troubleshooting

### Common Issues

#### 1. "Connection refused" to Ollama
```bash
# Check if Ollama is running
ollama list

# Start Ollama if not running
ollama serve

# Pull required model
ollama pull llama3.2:latest
```

#### 2. "Invalid API key" errors
```bash
# Verify API key format
echo $GROQ_API_KEY  # Should start with 'gsk_'
echo $OPENAI_API_KEY  # Should start with 'sk-'
echo $ANTHROPIC_API_KEY  # Should start with 'sk-ant-'
```

#### 3. Agent not found
```bash
# List available agents
curl http://localhost:8910/v1/models

# Check agent creation response for errors
curl -X POST http://localhost:8910/agents \
  -H "Content-Type: application/json" \
  -d '{"name": "test-agent", "supervisor": {"provider": "groq", "model": "llama3.2:3b"}}'
```

#### 4. Port already in use
```bash
# Find process using port 8910
lsof -i :8910

# Kill the process
kill -9 <PID>

# Or use a different port
PORT=8911 go run cmd/main.go
```

### Health Checks
```bash
# Service health
curl http://localhost:8910/health

# API responsiveness
curl http://localhost:8910/v1/models
```

### Logs
The service logs to stdout/stderr. For more detailed logging:
```bash
LOG_LEVEL=debug go run cmd/main.go
```

## Next Steps

### Explore More Features
- **Session Management**: Learn about persistent conversations
- **Streaming**: Understand real-time response streaming
- **MCP Integration**: Connect with MCP servers
- **Custom Tools**: Build custom agent capabilities

### Production Deployment
- **Database Configuration**: Set up persistent SQLite or external database
- **Security**: Configure authentication and authorization
- **Monitoring**: Set up logging and metrics collection
- **Scaling**: Deploy with load balancing for multiple instances

### Development
- **API Documentation**: Visit `/swagger/` for complete API reference
- **Code Examples**: Check the examples directory for more use cases
- **Contributing**: See CONTRIBUTING.md for development guidelines

## Support

- **Issues**: Report bugs on GitHub Issues
- **Discussions**: Join community discussions on GitHub
- **Documentation**: Full docs available in the `docs/` directory

Happy building with AI agents! 🤖