# SUSE AI Smart Agents API Service - Examples

This document provides practical examples for using the SUSE AI Smart Agents API service with various AI providers and configurations.

## Prerequisites

Before running these examples, ensure you have:

1. **Started the Smart Agents service:**
   ```bash
   cd suse-ai-up-smartagents && go run cmd/main.go
   ```

2. **Configured API keys** for your chosen providers:
   ```bash
   export GROQ_API_KEY="gsk_your_key_here"
   export OPENAI_API_KEY="sk-your_key_here"
   export ANTHROPIC_API_KEY="sk-ant-your_key_here"
   ```

## Basic Agent Creation and Usage

### 1. Create a Simple Agent
```bash
curl -X POST http://localhost:8910/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "basic-agent",
    "supervisor": {
      "provider": "groq",
      "api": "gsk_your_groq_key",
      "model": "llama-3.3-70b-versatile"
    },
    "worker": {
      "provider": "ollama",
      "url": "http://localhost:11434",
      "model": "llama3.2:latest"
    }
  }'
```

### 2. Test the Agent with Chat Completion
```bash
curl -X POST http://localhost:8910/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "basic-agent",
    "messages": [
      {"role": "user", "content": "Hello! Tell me about yourself."}
    ]
  }'
```

**Response:**
```json
{
  "id": "chatcmpl-123",
  "object": "chat.completion",
  "created": 1677652288,
  "model": "basic-agent",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello! I'm an AI assistant powered by a supervisor-worker architecture..."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 13,
    "completion_tokens": 87,
    "total_tokens": 100
  }
}
```

### 3. Use Streaming Responses
```bash
curl -X POST http://localhost:8910/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "basic-agent",
    "messages": [
      {"role": "user", "content": "Write a short story about AI."}
    ],
    "stream": true
  }'
```

**Streaming Response:**
```
data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"basic-agent","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"basic-agent","choices":[{"index":0,"delta":{"content":"Once"},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"basic-agent","choices":[{"index":0,"delta":{"content":" upon"},"finish_reason":null}]}

...
```

## Advanced Agent Configurations

### Code Review Agent
```bash
curl -X POST http://localhost:8910/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "code-reviewer",
    "supervisor": {
      "provider": "openai",
      "api": "sk-your_openai_key",
      "model": "gpt-4",
      "temperature": 0.3
    },
    "worker": {
      "provider": "ollama",
      "url": "http://localhost:11434",
      "model": "codellama:13b",
      "temperature": 0.1
    },
    "system_prompt": "You are an expert code reviewer. Focus on best practices, security, and maintainability."
  }'
```

### Creative Writing Agent
```bash
curl -X POST http://localhost:8910/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "creative-writer",
    "supervisor": {
      "provider": "anthropic",
      "api": "sk-ant-your_anthropic_key",
      "model": "claude-3-haiku-20240307",
      "temperature": 0.9
    },
    "worker": {
      "provider": "ollama",
      "url": "http://localhost:11434",
      "model": "llama2:13b-chat",
      "temperature": 0.8
    },
    "system_prompt": "You are a creative writing assistant. Help users develop stories, poems, and other creative content."
  }'
```

### Technical Documentation Agent
```bash
curl -X POST http://localhost:8910/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "tech-doc-writer",
    "supervisor": {
      "provider": "groq",
      "api": "gsk_your_groq_key",
      "model": "llama-3.3-70b-versatile",
      "temperature": 0.2
    },
    "worker": {
      "provider": "ollama",
      "url": "http://localhost:11434",
      "model": "technical-writer:latest",
      "temperature": 0.1
    },
    "system_prompt": "You are a technical documentation specialist. Create clear, accurate, and comprehensive documentation."
  }'
```

## Using Different AI Providers

### OpenAI + Ollama Combination
```bash
curl -X POST http://localhost:8910/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "openai-ollama-agent",
    "supervisor": {
      "provider": "openai",
      "api": "sk-your_openai_key",
      "model": "gpt-4-turbo-preview"
    },
    "worker": {
      "provider": "ollama",
      "url": "http://localhost:11434",
      "model": "mistral:7b-instruct"
    }
  }'
```

### Anthropic + Groq Combination
```bash
curl -X POST http://localhost:8910/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "anthropic-groq-agent",
    "supervisor": {
      "provider": "anthropic",
      "api": "sk-ant-your_anthropic_key",
      "model": "claude-3-sonnet-20240229"
    },
    "worker": {
      "provider": "groq",
      "api": "gsk_your_groq_key",
      "model": "mixtral-8x7b-32768"
    }
  }'
```

### All Local Models (Ollama Only)
```bash
curl -X POST http://localhost:8910/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "local-only-agent",
    "supervisor": {
      "provider": "ollama",
      "url": "http://localhost:11434",
      "model": "llama3.2:3b"
    },
    "worker": {
      "provider": "ollama",
      "url": "http://localhost:11434",
      "model": "codellama:7b"
    }
  }'
```

## Agent Management Examples

### List All Agents
```bash
curl http://localhost:8910/v1/models
```

**Response:**
```json
{
  "object": "list",
  "data": [
    {
      "id": "basic-agent",
      "object": "model",
      "created": 1677652288,
      "owned_by": "smartagents"
    },
    {
      "id": "code-reviewer",
      "object": "model",
      "created": 1677652290,
      "owned_by": "smartagents"
    }
  ]
}
```

### Get Agent Details
```bash
curl http://localhost:8910/agents/basic-agent
```

**Response:**
```json
{
  "id": "agent_123",
  "name": "basic-agent",
  "supervisor": {
    "provider": "groq",
    "model": "llama-3.3-70b-versatile"
  },
  "worker": {
    "provider": "ollama",
    "model": "llama3.2:latest"
  },
  "created_at": "2025-10-28T12:00:00Z",
  "status": "active",
  "usage_stats": {
    "total_requests": 42,
    "total_tokens": 12580
  }
}
```

### Update Agent Configuration
```bash
curl -X PUT http://localhost:8910/agents/basic-agent \
  -H "Content-Type: application/json" \
  -d '{
    "supervisor": {
      "temperature": 0.7
    },
    "worker": {
      "temperature": 0.8
    }
  }'
```

### Delete Agent
```bash
curl -X DELETE http://localhost:8910/agents/basic-agent
```

## MCP Registry Examples

### Upload Custom MCP Servers
```bash
curl -X POST http://localhost:8910/registry/upload/bulk \
  -H "Content-Type: application/json" \
  -d '[
    {
      "name": "filesystem-server",
      "description": "Provides file system access capabilities",
      "version": "1.0.0",
      "packages": [
        {
          "registryType": "npm",
          "identifier": "@modelcontextprotocol/server-filesystem",
          "transport": {
            "type": "stdio"
          },
          "environmentVariables": [
            {
              "name": "ALLOWED_DIRS",
              "description": "Comma-separated list of allowed directories",
              "default": "/tmp"
            }
          ]
        }
      ],
      "_meta": {
        "author": "anthropic",
        "tags": ["filesystem", "utility"]
      }
    }
  ]'
```

### Browse Available MCP Servers
```bash
# Get all servers
curl http://localhost:8910/registry/browse

# Filter by transport type
curl "http://localhost:8910/registry/browse?transport=stdio"

# Search by name
curl "http://localhost:8910/registry/browse?query=filesystem"

# Filter by registry type
curl "http://localhost:8910/registry/browse?registryType=npm"
```

### Upload Single MCP Server
```bash
curl -X POST http://localhost:8910/registry/upload \
  -H "Content-Type: application/json" \
  -d '{
    "id": "my-custom-server",
    "name": "My Custom Server",
    "description": "A custom MCP server for specialized tasks",
    "packages": [{
      "registryType": "npm",
      "identifier": "@myorg/mcp-server",
      "transport": {"type": "stdio"},
      "environmentVariables": [{
        "name": "API_KEY",
        "description": "API key for external service"
      }]
    }],
    "_meta": {
      "author": "myorg",
      "version": "1.0.0"
    }
  }'
```

## OpenAI-Compatible Client Examples

### Python Client
```python
import openai

# Initialize client
client = openai.OpenAI(
    base_url="http://localhost:8910/v1",
    api_key="dummy"  # Not required for local setup
)

# Basic chat completion
response = client.chat.completions.create(
    model="basic-agent",
    messages=[
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": "Explain quantum computing in simple terms."}
    ]
)

print(response.choices[0].message.content)

# Streaming response
response = client.chat.completions.create(
    model="creative-writer",
    messages=[{"role": "user", "content": "Write a haiku about AI."}],
    stream=True
)

for chunk in response:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="")
```

### JavaScript/Node.js Client
```javascript
const OpenAI = require('openai');

const client = new OpenAI({
  baseURL: 'http://localhost:8910/v1',
  apiKey: 'dummy'  // Not required for local setup
});

// Basic usage
async function chat() {
  const response = await client.chat.completions.create({
    model: 'code-reviewer',
    messages: [
      { role: 'user', content: 'Review this JavaScript function...' }
    ]
  });

  console.log(response.choices[0].message.content);
}

// Streaming usage
async function streamChat() {
  const stream = await client.chat.completions.create({
    model: 'creative-writer',
    messages: [{ role: 'user', content: 'Tell me a story.' }],
    stream: true
  });

  for await (const chunk of stream) {
    process.stdout.write(chunk.choices[0]?.delta?.content || '');
  }
}
```

### cURL Examples
```bash
# Basic chat completion
curl -X POST http://localhost:8910/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "basic-agent",
    "messages": [
      {"role": "user", "content": "What is the capital of France?"}
    ]
  }'

# Multi-turn conversation
curl -X POST http://localhost:8910/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "basic-agent",
    "messages": [
      {"role": "user", "content": "What is machine learning?"},
      {"role": "assistant", "content": "Machine learning is a subset of AI..."},
      {"role": "user", "content": "Can you give me a simple example?"}
    ]
  }'

# With parameters
curl -X POST http://localhost:8910/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "creative-writer",
    "messages": [{"role": "user", "content": "Write a poem."}],
    "temperature": 0.9,
    "max_tokens": 200,
    "top_p": 0.9
  }'
```

## Example Chat Client Usage

### Terminal Chat Interface
```bash
# Navigate to examples
cd examples/smartagents

# Run with basic agent
go run main.go basic-agent http://localhost:8910

# Run with code reviewer
go run main.go code-reviewer http://localhost:8910
```

### Sample Interaction Output
```
🤖 Rancher SA Chat Client
Agent: code-reviewer
API: http://localhost:8910
Type 'quit' or 'exit' to end the conversation
=================================================

👤 You: Can you review this Python function?

def calculate_average(numbers):
    total = 0
    for num in numbers:
        total += num
    return total / len(numbers)

🧠 Supervisor: The user wants a code review. I should analyze this function for:
- Correctness
- Efficiency
- Best practices
- Edge cases

👷 Worker: Reviewing the calculate_average function:
- Uses basic loop accumulation
- Handles division by zero if empty list
- No input validation
- Could use built-in sum() function

🤖 Assistant: Here's my review of your `calculate_average` function:

**Strengths:**
- Simple and readable implementation
- Correct basic logic

**Suggestions for improvement:**
1. **Input validation**: Add check for empty list to prevent ZeroDivisionError
2. **Use built-in functions**: Consider `sum(numbers) / len(numbers)` for better performance
3. **Type hints**: Add type annotations for better code documentation
4. **Edge cases**: Handle non-numeric inputs

**Improved version:**
```python
from typing import List, Union

def calculate_average(numbers: List[Union[int, float]]) -> float:
    if not numbers:
        raise ValueError("Cannot calculate average of empty list")
    return sum(numbers) / len(numbers)
```

⏱️  Request completed in 4.12s
📊 Tokens used: 387 (Supervisor: 145, Worker: 242)
```

## Advanced Usage Patterns

### Custom System Prompts
```bash
curl -X POST http://localhost:8910/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "expert-consultant",
    "supervisor": {
      "provider": "openai",
      "model": "gpt-4",
      "temperature": 0.2
    },
    "worker": {
      "provider": "anthropic",
      "model": "claude-3-sonnet-20240229",
      "temperature": 0.1
    },
    "system_prompt": "You are a senior software architect with 15 years of experience. Provide detailed, practical advice with real-world examples. Always consider scalability, maintainability, and security implications."
  }'
```

### Batch Processing
```bash
# Process multiple requests
for query in "What is AI?" "Explain neural networks" "Future of ML"; do
  curl -X POST http://localhost:8910/v1/chat/completions \
    -H "Content-Type: application/json" \
    -d "{\"model\": \"basic-agent\", \"messages\": [{\"role\": \"user\", \"content\": \"$query\"}]}" \
    -s | jq '.choices[0].message.content'
  echo "---"
done
```

### Error Handling
```bash
# Test error responses
curl -X POST http://localhost:8910/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nonexistent-agent",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

**Error Response:**
```json
{
  "error": {
    "message": "Agent 'nonexistent-agent' not found",
    "type": "invalid_request_error",
    "code": 404
  }
}
```

## Integration Examples

### Web Application Integration
```javascript
// Frontend integration example
async function sendMessage(agentName, message) {
  const response = await fetch('http://localhost:8910/v1/chat/completions', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      model: agentName,
      messages: [{ role: 'user', content: message }],
      stream: true
    })
  });

  const reader = response.body.getReader();
  const decoder = new TextDecoder();

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    const chunk = decoder.decode(value);
    const lines = chunk.split('\n');

    for (const line of lines) {
      if (line.startsWith('data: ')) {
        const data = line.slice(6);
        if (data === '[DONE]') return;

        try {
          const parsed = JSON.parse(data);
          const content = parsed.choices[0]?.delta?.content;
          if (content) {
            // Append to UI
            appendToChat(content);
          }
        } catch (e) {
          console.error('Parse error:', e);
        }
      }
    }
  }
}
```

### API Monitoring
```bash
# Monitor agent usage
curl http://localhost:8910/agents | jq '.agents[].usage_stats'

# Health check
curl http://localhost:8910/health

# Service metrics (if enabled)
curl http://localhost:8910/metrics
```

These examples demonstrate the flexibility and power of the SUSE AI Smart Agents API service for various use cases and integration patterns.