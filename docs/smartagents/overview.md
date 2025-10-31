# SUSE AI Smart Agents API Service - Overview

**Note:** This documentation is for the SmartAgents service, which has been moved to a separate repository at `~/Documents/innovation/suse-ai-up-smartagents`.

The SUSE AI Smart Agents API service (SAAS) is a powerful API server that provides smart agent capabilities with supervisor-worker orchestration, supporting multiple AI providers and real-time streaming.

## Features

### Core Capabilities
- **Supervisor-Worker Orchestration**: Intelligent task planning and execution with specialized AI roles
- **Multi-Provider Support**: Seamless integration with various AI providers (OpenAI, Groq, Anthropic, Ollama, etc.)
- **Real-time Streaming**: Live streaming of agent interactions and responses
- **Agent Management**: Create, configure, and manage multiple AI agents
- **MCP Registry Integration**: Comprehensive MCP server registry with discovery and management

### Agent Architecture

Agents combine **supervisor** and **worker** roles for optimal AI task execution:

- **Supervisor**: High-level reasoning and task planning (e.g., Groq, OpenAI)
  - Strategic thinking and problem decomposition
  - Task prioritization and workflow planning
  - Quality assurance and final response synthesis

- **Worker**: Task execution and detailed analysis (e.g., Ollama, local models)
  - Detailed implementation and code generation
  - Data processing and analysis
  - Specialized domain expertise

### Supported AI Providers

#### Cloud Providers
- **OpenAI**: GPT-4, GPT-3.5 Turbo, and other models
- **Anthropic**: Claude models with advanced reasoning
- **Groq**: Fast inference with Llama models

#### Local Providers
- **Ollama**: Run open-source models locally
- **Custom Endpoints**: Support for any OpenAI-compatible API

### API Endpoints

#### Chat Completions
- `POST /v1/chat/completions` - OpenAI-compatible chat completions with streaming
- Supports real-time streaming of supervisor-worker interactions
- Compatible with existing OpenAI client libraries

#### Agent Management
- `GET /v1/models` - List available agents (OpenAI-compatible)
- `POST /agents` - Create new agents
- `GET /agents` - List all agents
- `GET /agents/{id}` - Get agent details
- `PUT /agents/{id}` - Update agent configuration
- `DELETE /agents/{id}` - Delete agent

#### MCP Registry
- `GET /registry` - Browse MCP servers
- `POST /registry/upload` - Upload MCP server configurations
- `POST /registry/upload/bulk` - Bulk upload multiple configurations

### Real-time Streaming

The service provides live streaming of agent interactions:

```
User Query → Supervisor → Worker → Supervisor → Final Response
     ↓          ↓          ↓          ↓          ↓
  Streaming   Planning   Execution  Synthesis  Complete
```

#### Streaming Features
- **Colored Output**: Different colors for user, assistant, worker, and supervisor messages
- **Performance Monitoring**: Request timing and token usage statistics
- **Interactive Display**: Real-time updates during agent processing
- **Usage Statistics**: Detailed token breakdowns by provider and agent role

### Agent Configuration

#### Basic Agent Structure
```json
{
  "name": "my-agent",
  "supervisor": {
    "provider": "groq",
    "api": "gsk_...",
    "model": "llama-3.3-70b-versatile"
  },
  "worker": {
    "provider": "ollama",
    "url": "http://localhost:11434",
    "model": "llama3.2:latest"
  }
}
```

#### Advanced Configuration
```json
{
  "name": "advanced-agent",
  "supervisor": {
    "provider": "openai",
    "api": "sk-...",
    "model": "gpt-4",
    "temperature": 0.7,
    "max_tokens": 2000
  },
  "worker": {
    "provider": "anthropic",
    "api": "sk-ant-...",
    "model": "claude-3-sonnet-20240229",
    "temperature": 0.3
  },
  "system_prompt": "Custom system instructions...",
  "max_iterations": 5
}
```

### MCP Registry Integration

#### Registry Features
- **Bulk Upload**: Upload MCP server configurations from JSON/YAML files
- **Advanced Search**: Search and filter MCP servers by provider, transport type, and metadata
- **Auto-Generation**: Automatic adapter creation from registry entries
- **Multi-Source Support**: Combine official registry with custom/private catalogs

#### Registry API Usage
```bash
# Upload custom MCP servers
curl -X POST http://localhost:8910/registry/upload/bulk \
  -H "Content-Type: application/json" \
  -d @my-mcp-servers.json

# Search available servers
curl "http://localhost:8910/registry/browse?transport=stdio&provider=anthropic"
```

### Example Chat Client

The service includes a feature-rich terminal chat client:

#### Features
- **Real-time Streaming**: Live display of supervisor-worker interactions
- **Colored Output**: Visual distinction between different agent roles
- **Usage Statistics**: Token usage breakdowns and performance metrics
- **Interactive Interface**: Clean chat interface with command history
- **Multi-agent Support**: Switch between different configured agents

#### Usage
```bash
# Start the API server
go run cmd/main.go

# Configure an agent
curl -X POST http://localhost:8910/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-agent",
    "supervisor": {"provider": "groq", "api": "gsk_...", "model": "llama-3.3-70b-versatile"},
    "worker": {"provider": "ollama", "url": "http://localhost:11434", "model": "llama3.2:latest"}
  }'

# Run the chat client
cd examples/smartagents && go run main.go my-agent http://localhost:8910
```

#### Sample Interaction
```
🤖 Rancher SA Chat Client
Agent: my-agent
API: http://localhost:8910
Type 'quit' or 'exit' to end the conversation
=================================================

👤 You: Explain how neural networks work

🤖 Assistant: Neural networks are computational models inspired by biological neural networks...

🧠 Supervisor: The user asked about neural networks. I should break this down into key concepts...

👷 Worker: Let me provide a detailed technical explanation of neural network architecture...

🤖 Assistant: To explain how neural networks work, let's break it down into key components...

⏱️  Request completed in 3.45s
📊 Tokens used: 1247 (Supervisor: 423, Worker: 824)
```

### Architecture Overview

#### Service Components
```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   API Layer     │    │  Agent Engine   │    │ Registry Service│
│                 │    │                 │    │                 │
│ • REST API      │    │ • Supervisor     │    │ • MCP Discovery │
│ • WebSocket     │    │ • Worker         │    │ • Server Mgmt   │
│ • Streaming     │    │ • Orchestration  │    │ • Auto-adapters │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                    ┌─────────────────┐
                    │   AI Providers  │
                    │                 │
                    │ • OpenAI        │
                    │ • Anthropic     │
                    │ • Groq          │
                    │ • Ollama        │
                    │ • Custom APIs   │
                    └─────────────────┘
```

#### Data Flow
1. **Request Reception**: API receives chat completion request
2. **Agent Selection**: Route to appropriate agent based on model name
3. **Supervisor Processing**: High-level task analysis and planning
4. **Worker Execution**: Detailed task execution and content generation
5. **Response Synthesis**: Supervisor reviews and finalizes response
6. **Streaming Output**: Real-time delivery of response with role indicators

### Integration Examples

#### OpenAI Client Compatibility
```python
import openai

client = openai.OpenAI(
    base_url="http://localhost:8910/v1",
    api_key="dummy"  # Not required for local setup
)

response = client.chat.completions.create(
    model="my-agent",  # Agent name as model
    messages=[{"role": "user", "content": "Hello!"}],
    stream=True
)

for chunk in response:
    print(chunk.choices[0].delta.content, end="")
```

#### cURL Examples
```bash
# List available agents
curl http://localhost:8910/v1/models

# Chat completion
curl -X POST http://localhost:8910/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "my-agent",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'

# Create agent
curl -X POST http://localhost:8910/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "code-reviewer",
    "supervisor": {"provider": "openai", "model": "gpt-4"},
    "worker": {"provider": "ollama", "model": "codellama"}
  }'
```

### Performance & Monitoring

#### Metrics
- **Response Times**: End-to-end request processing time
- **Token Usage**: Breakdown by provider and agent role
- **Success Rates**: API success and error rates
- **Concurrent Users**: Active connections and sessions

#### Logging
- **Structured Logs**: JSON-formatted logs with context
- **Performance Tracing**: Request tracing through supervisor-worker pipeline
- **Error Tracking**: Detailed error logging with stack traces
- **Audit Trail**: Complete record of agent interactions

### Security Considerations

#### API Security
- **Authentication**: API key validation for cloud providers
- **Rate Limiting**: Request rate limiting to prevent abuse
- **Input Validation**: Comprehensive input sanitization
- **Output Filtering**: Content filtering for sensitive information

#### Data Protection
- **No Data Persistence**: Conversations not stored by default
- **Provider Compliance**: Adherence to provider data usage policies
- **Encryption**: TLS encryption for all external communications
- **Access Controls**: Configurable access controls for agent management

### Deployment Options

#### Local Development
- **Standalone Mode**: Run with local Ollama instance
- **Docker Compose**: Full stack with proxy and registry
- **Kubernetes**: Helm charts for cloud deployment

#### Production Deployment
- **High Availability**: Load balancing and redundancy
- **Monitoring**: Integration with Prometheus/Grafana
- **Scaling**: Horizontal scaling for increased load
- **Backup**: Configuration and registry backup procedures

### Future Enhancements

#### Planned Features
- **Advanced Orchestration**: Multi-agent collaboration workflows
- **Custom Tools**: User-defined tools and integrations
- **Fine-tuning**: Model fine-tuning capabilities
- **Analytics**: Advanced usage analytics and insights
- **Plugin System**: Extensible plugin architecture

This service provides a powerful foundation for building intelligent AI applications with sophisticated agent orchestration and seamless integration with multiple AI providers.