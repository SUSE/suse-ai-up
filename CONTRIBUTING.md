# How to contribute

We're thrilled you're interested in contributing to SUSE AI Universal Proxy! This project aims to provide a robust reverse proxy and management layer for Model Context Protocol (MCP) servers. Your contributions help make AI development more accessible and scalable.

If you haven't already, check out our [GitHub Issues](https://github.com/suse/suse-ai-up/issues) for current discussions and feature requests. We want you working on things you're excited about.

Here are some important resources:

  * [SUSE AI Universal Proxy README](README.md) for an overview of the project
  * [Our roadmap](https://github.com/suse/suse-ai-up/projects) for planned features and priorities
  * [GitHub Issues](https://github.com/suse/suse-ai-up/issues) for bug reports and feature requests
  * Mailing list: Join our [developer discussions](https://github.com/suse/suse-ai-up/discussions) on GitHub

## Testing

We use Go's built-in testing framework for unit tests and integration tests. Please write comprehensive tests for new code you create. For the smart agents service, we also have test data and mock setups.

Run tests with:
```bash
go test ./...
```

For specific services:
- Proxy: `cd suse-ai-up-proxy && go test ./...`

## Submitting changes

To contribute to this project, follow these steps:

1. **Fork the repository**: First, fork the repo to your own GitHub account.

2. **Do not create a new branch**: Work directly on the main branch of your fork.

3. **Open an issue**: Create a new issue in the original repository with the label "epic". The issue must clearly describe:
   - The problem you're addressing
   - Why it's important
   - How you think you may solve it

4. **Wait for approval**: Once the issue is approved by maintainers, you may proceed.

5. **Submit a Pull Request**: After approval, submit your work as a PR from your fork's main branch, linking it to the approved issue.

Please follow our coding conventions (below) and make sure all of your commits are atomic (one feature per commit).

Always write a clear log message for your commits. One-line messages are fine for small changes, but bigger changes should look like this:

    $ git commit -m "A brief summary of the commit
    >
    > A paragraph describing what changed and its impact."

## Coding conventions

Start reading our code and you'll get the hang of it. We optimize for readability and maintainability:

  * We use `go fmt` for consistent formatting
  * We follow Go naming conventions (camelCase for unexported, PascalCase for exported)
  * We use meaningful variable and function names
  * We add comments for exported functions and types using Go doc conventions
  * We avoid global variables; prefer dependency injection
  * We handle errors properly, not ignoring them
  * We write tests for all public APIs
  * This is open source software. Consider the people who will read your code, and make it look nice for them. It's sort of like driving a car: Perhaps you love doing donuts when you're alone, but with passengers the goal is to make the ride as smooth as possible.
   * For Kubernetes deployments, follow Helm chart best practices
   * Use structured logging with appropriate log levels

## Plugin Development Guidelines

The SUSE AI Universal Proxy supports a plugin architecture that enables seamless integration of specialized AI services. When developing plugins, follow these guidelines to ensure compatibility and maintainability.

### Service Registration

**Automatic Registration:**
- Set the `PROXY_URL` environment variable to enable automatic registration
- Implement graceful handling when proxy is unavailable
- Use unique, descriptive service IDs (consider including timestamps or UUIDs)

**Manual Registration:**
```go
// Example service registration payload
registration := PluginRegistration{
    ServiceID:   "my-service-" + generateUniqueID(),
    ServiceType: "smartagents", // or "registry", "virtualmcp"
    ServiceURL:  "http://localhost:8080",
    Version:     "1.0.0",
    Capabilities: []Capability{
        {
            Path:        "/api/v1/*",
            Methods:     []string{"GET", "POST"},
            Description: "REST API endpoints",
        },
    },
}
```

### Capability Declaration

**Path Patterns:**
- Use specific paths like `/v1/models` for exact matches
- Use wildcards like `/api/v1/*` for flexible routing
- Avoid overly broad patterns like `/*` that might conflict with other services
- Document path parameters and expected formats

**HTTP Methods:**
- Declare only the methods your service actually supports
- Include `OPTIONS` if your service handles CORS preflight requests
- Consider `HEAD` requests for health checks

**Best Practices:**
```go
// Good: Specific, well-documented capabilities
capabilities := []Capability{
    {
        Path:        "/v1/chat/completions",
        Methods:     []string{"POST"},
        Description: "OpenAI-compatible chat completions with streaming",
    },
    {
        Path:        "/v1/models",
        Methods:     []string{"GET"},
        Description: "List available AI models",
    },
}

// Avoid: Overly broad or undocumented capabilities
badCapabilities := []Capability{
    {
        Path:        "/*",
        Methods:     []string{"GET", "POST", "PUT", "DELETE"},
        Description: "All endpoints", // Too vague
    },
}
```

### Health Check Implementation

**Health Endpoint:**
- Implement a `/health` endpoint that returns JSON status
- Include response time and last check timestamp
- Return appropriate HTTP status codes (200 for healthy, 503 for unhealthy)

**Example Health Response:**
```json
{
  "status": "healthy",
  "message": "Service is responding normally",
  "timestamp": "2025-10-28T12:36:28Z",
  "version": "1.0.0",
  "uptime": "2h30m45s"
}
```

**Health Check Logic:**
```go
func (s *MyService) healthHandler(w http.ResponseWriter, r *http.Request) {
    health := map[string]interface{}{
        "status":    "healthy",
        "message":   "Service is responding normally",
        "timestamp": time.Now().Format(time.RFC3339),
        "version":   s.version,
        "uptime":    time.Since(s.startTime).String(),
    }

    // Check dependencies
    if err := s.checkDatabase(); err != nil {
        health["status"] = "unhealthy"
        health["message"] = fmt.Sprintf("Database check failed: %v", err)
        w.WriteHeader(http.StatusServiceUnavailable)
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(health)
}
```

### Testing Guidelines

**Unit Tests:**
- Test service registration and deregistration
- Test capability matching and routing logic
- Test health check endpoints
- Mock external dependencies

**Integration Tests:**
- Test with actual proxy service
- Verify automatic registration works
- Test dynamic routing through proxy
- Test error scenarios and recovery

**Example Test Structure:**
```go
func TestPluginRegistration(t *testing.T) {
    // Test automatic registration
    service := NewMyService()
    service.SetProxyURL("http://localhost:8911")

    // Verify registration
    resp, err := http.Get("http://localhost:8911/api/v1/plugins/services")
    // Assert service appears in list
}

func TestCapabilityRouting(t *testing.T) {
    // Test that requests are routed correctly
    resp, err := http.Get("http://localhost:8911/v1/models")
    // Assert response comes from correct service
}

func TestHealthChecks(t *testing.T) {
    // Test health endpoint
    resp, err := http.Get("http://localhost:8080/health")
    // Assert proper health response
}
```

### Code Structure

**Recommended Plugin Structure:**
```
my-plugin/
├── cmd/
│   └── main.go              # Service entry point
├── internal/
│   ├── service/
│   │   ├── service.go       # Core service logic
│   │   └── plugin.go        # Plugin registration logic
│   └── config/
│       └── config.go        # Configuration management
├── pkg/
│   └── api/                 # Public API definitions
├── test/
│   ├── integration_test.go  # Integration tests
│   └── mock/               # Test mocks
├── go.mod
├── README.md
└── Dockerfile
```

**Plugin Interface:**
```go
type PluginService interface {
    Register(ctx context.Context, proxyURL string) error
    Unregister(ctx context.Context) error
    Health(ctx context.Context) (HealthStatus, error)
    Capabilities() []Capability
}

type Capability struct {
    Path        string   `json:"path"`
    Methods     []string `json:"methods"`
    Description string   `json:"description"`
}
```

### Error Handling

**Plugin-Specific Errors:**
- Use structured error types with context
- Include service ID and operation in error messages
- Log errors with appropriate levels

**Example Error Handling:**
```go
type PluginError struct {
    ServiceID string
    Operation string
    Message   string
    Cause     error
}

func (e PluginError) Error() string {
    return fmt.Sprintf("plugin %s %s failed: %s", e.ServiceID, e.Operation, e.Message)
}

func (s *MyService) Register(ctx context.Context, proxyURL string) error {
    req, err := http.NewRequestWithContext(ctx, "POST", proxyURL+"/api/v1/plugins/register", bytes.NewReader(data))
    if err != nil {
        return PluginError{
            ServiceID: s.serviceID,
            Operation: "register",
            Message:   "failed to create registration request",
            Cause:     err,
        }
    }
    // ... rest of registration logic
}
```

### Security Considerations

- Validate all input data from proxy
- Implement proper authentication if required
- Use HTTPS for production deployments
- Log security-relevant events
- Follow principle of least privilege

### Deployment

**Container Best Practices:**
- Use multi-stage Docker builds
- Include health check in Dockerfile
- Set appropriate resource limits
- Use non-root user

**Environment Variables:**
- Document all required environment variables
- Provide sensible defaults where possible
- Use `PROXY_URL` for proxy integration

Thanks,
SUSE AI Team