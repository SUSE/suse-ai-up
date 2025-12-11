# AGENTS.md

## Build Commands
- **Go**: `go build -o service ./cmd`
- **Python**: `cd examples/local-mcp && pip install -r requirements.txt`
- **Swagger**: `swag init -g cmd/service/main.go`

## Test Commands
- **Go**: `go test ./...` (all tests)
- **Go**: `go test -run TestName ./pkg/...` (single test)
- **Go**: `go test -v ./pkg/plugins/...` (verbose output for specific package)

## Lint Commands
- **Go**: `gofmt -d .` (check formatting)
- **Go**: `go vet ./...` (static analysis)
- **Go**: `go fmt ./...` (auto-format code)
- **Python**: No specific linter configured

## Code Style Guidelines

### Go
- **Formatting**: Use `go fmt` for consistent formatting
- **Naming**: camelCase for unexported identifiers, PascalCase for exported
- **Variables**: Use meaningful, descriptive names; avoid abbreviations
- **Functions**: Add comments for exported functions using Go doc conventions
- **Error Handling**: Always handle errors properly, never ignore them
- **Logging**: Use structured logging with appropriate log levels
- **Architecture**: Avoid global variables; prefer dependency injection
- **Testing**: Write tests for all public APIs using Go's testing framework
- **Imports**: Group imports (standard library, third-party, local) with blank lines
- **Types**: Use interfaces for dependency injection and testability
- **Concurrency**: Use channels and goroutines appropriately; document synchronization

### Python
- **Style**: Follow PEP 8 conventions
- **Naming**: Use descriptive names; snake_case for variables/functions, PascalCase for classes
- **Imports**: Import specific functions/classes, not entire modules when possible
- **Error Handling**: Use try/except blocks appropriately; raise meaningful exceptions
- **Documentation**: Add docstrings for functions and classes
- **Structure**: Keep code simple and clean; avoid deep nesting

### General
- **Comments**: Add comments for complex logic and exported APIs
- **Dependencies**: Check existing codebase before adding new libraries
- **Security**: Never expose secrets/keys; validate all inputs
- **Kubernetes**: Follow Helm chart best practices for deployments
- **Plugin Architecture**: Use the established plugin interface for extensibility

## Environment Variables
- **AUTH_MODE**: Set to "oauth" for OAuth authentication, "development" (default) for development mode
- **PORT**: Server port (default: 8911)
- **HOST**: Server host (default: localhost)
- **API_KEY**: API key for authentication
- **SMARTAGENTS_ENABLED**: Enable/disable smartagents service (default: true)
- **REGISTRY_ENABLED**: Enable/disable registry service (default: true)
- **REGISTRY_ENABLE_OFFICIAL**: Enable official registry sources (default: true)

No Cursor or Copilot rules found in the repository.