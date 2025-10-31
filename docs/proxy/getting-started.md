# SUSE AI Universal Proxy - Getting Started

This guide will help you get the SUSE AI Universal Proxy up and running locally for development and testing.

## Prerequisites

### System Requirements
- Go 1.23 or later
- Docker Desktop with Kubernetes enabled
- API keys for AI providers (optional, for smart agents)

### Local Development Environment
1. [Install Go 1.24](https://golang.org/dl/)
2. [Install Docker Desktop](https://docs.docker.com/desktop/)
3. [Install and turn on Kubernetes](https://docs.docker.com/desktop/features/kubernetes/#install-and-turn-on-kubernetes)

## Quick Start with Curl

### 1. Start the proxy service
```bash
cd suse-ai-up-proxy && go run cmd/service/main.go
```

### 2. Start SmartAgents with proxy registration
```bash
cd suse-ai-up-smartagents && PROXY_URL=http://localhost:8911 go run cmd/main.go
```

### 3. Check registered services
```bash
curl http://localhost:8911/plugins/services
```

### 4. Test dynamic routing (routes to SmartAgents)
```bash
curl http://localhost:8911/v1/models
curl http://localhost:8911/agents
```

### 5. Check service health
```bash
curl http://localhost:8911/plugins/services/smartagents-*/health
```

## Local Deployment

### 1. Prepare Local Development Environment
- [Install Go 1.24](https://golang.org/dl/)
- [Install Docker Desktop](https://docs.docker.com/desktop/)
- [Install and turn on Kubernetes](https://docs.docker.com/desktop/features/kubernetes/#install-and-turn-on-kubernetes)

### 2. Run Local Docker Registry
```sh
docker run -d -p 5000:5000 --name registry registry:2.7
```

### 3. Build & Publish MCP Server Images
Build and push the MCP server images to your local registry (`localhost:5000`).
```sh
docker build -f examples/Dockerfile examples -t localhost:5000/mcp-example:1.0.0
docker push localhost:5000/mcp-example:1.0.0
```

### 4. Build & Publish SUSE AI Universal Proxy
Build the Go service:
```sh
cd mcp-gateway && go build ./cmd/service
```

Build and push the Docker image:
```sh
docker build -t localhost:5000/mcp-gateway:latest ./mcp-gateway
docker push localhost:5000/mcp-gateway:latest
```

### 5. Deploy SUSE AI Universal Proxy to Kubernetes Cluster
Apply the deployment manifests:
```sh
kubectl apply -f deployment/k8s/local-deployment.yml
```

### 6. Enable Port Forwarding
Forward the gateway service port:
```sh
kubectl port-forward -n adapter svc/mcpgateway-service 8000:8000
```

### 7. Test the API - MCP Server Management
- **Interactive API Documentation**: Visit `http://localhost:8911/docs` to access the Swagger UI for interactive API testing and documentation.
- **OpenAPI Specification**: The OpenAPI spec is available at `http://localhost:8911/swagger/doc.json` or import from `docs/swagger.json` into tools like [Postman](https://www.postman.com/), [Bruno](https://www.usebruno.com/), or [Swagger Editor](https://editor.swagger.io/).

- Send a request to create a new adapter resource:
  ```http
  POST http://localhost:8911/adapters
  Content-Type: application/json
  ```
  ```json
  {
     "name": "mcp-example",
     "imageName": "mcp-example",
     "imageVersion": "1.0.0",
     "description": "test"
  }
  ```

### 8. Test the API - MCP Server Access
- After deploying the MCP server, use a client like [VS Code](https://code.visualstudio.com/) to test the connection. Refer to the guide: [Use MCP servers in VS Code](https://code.visualstudio.com/docs/copilot/chat/mcp-servers).
  > **Note:** Ensure VSCode is up to date to access the latest MCP features.

  - To connect to the deployed `mcp-example` server, use:
    - `http://localhost:8911/adapters/mcp-example/mcp` (Streamable HTTP)

  Sample `.vscode/mcp.json` that connects to the `mcp-example` server
  ```json
  {
    "servers": {
      "mcp-example": {
        "url": "http://localhost:8000/adapters/mcp-example/mcp",
      }
    }
  }
  ```

  - For other servers:
    - `http://localhost:8000/adapters/{name}/mcp` (Streamable HTTP)
    - `http://localhost:8000/adapters/{name}/sse` (SSE)

### 9. Clean the Environment
To remove all deployed resources, delete the Kubernetes namespace:
```sh
kubectl delete namespace adapter
```

## Local Development Workflow

### Setting Up the Proxy Service
1. Navigate to the proxy directory: `cd suse-ai-up-proxy`
2. Install dependencies and build: `cd mcp-gateway && go build ./cmd/service`
3. Start local registry: `docker run -d -p 5000:5000 --name registry registry:2.7`
4. Build and deploy MCP servers: Follow the detailed steps in `suse-ai-up-proxy/README.md` for local Kubernetes deployment
5. Access the API at `http://localhost:8001` with Swagger docs at `/docs`

### Development Workflow
- Run tests: `go test ./...` in each service directory
- Format code: `go fmt ./...`
- Test registry integration: See [Registry Documentation](docs/registry.md#integration-examples) for testing registry features
- Use the provided Helm charts for testing deployments
- Refer to individual service READMEs for detailed setup and examples

## Plugin Registration Example

```bash
# Register a service manually
curl -X POST http://localhost:8911/plugins/register \
  -H "Content-Type: application/json" \
  -d '{
    "service_id": "my-service",
    "service_type": "smartagents",
    "service_url": "http://localhost:8910",
    "version": "1.0.0",
    "capabilities": [
      {
        "path": "/api/v1/*",
        "methods": ["GET", "POST"],
        "description": "My API endpoints"
      }
    ]
  }'
```

## Troubleshooting

### Common Issues

1. **Port conflicts**: Ensure ports 8911 (proxy) and 8910 (smartagents) are available
2. **Kubernetes not running**: Verify Docker Desktop Kubernetes is enabled
3. **Registry not accessible**: Check that the local registry is running on port 5000
4. **Service registration fails**: Verify PROXY_URL environment variable is set correctly

### Health Checks
- Proxy health: `curl http://localhost:8911/health`
- Service registration status: `curl http://localhost:8911/plugins/services`

### Logs
- Proxy logs: Check the terminal where the proxy service is running
- Kubernetes logs: `kubectl logs -n adapter deployment/mcp-gateway`