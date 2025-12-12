# AGENTS.md

## Architecture Overview

SUSE AI Uniproxy uses a unified service architecture where all MCP services (uniproxy, registry, discovery, plugins) run as a single binary with separate logging. The main CLI commands are:

- `suse-ai-up uniproxy` - Run the comprehensive MCP proxy service
- `suse-ai-up all` - Run all services together with separate logging
- `suse-ai-up health` - Check service health

Individual service binaries (`suse-ai-up-discovery`, `suse-ai-up-registry`, `suse-ai-up-plugins`) are available for separate deployment scenarios.

## Build Commands
- **Go**: `go build -o suse-ai-up ./cmd` (builds unified binary with all services)
- **Python**: `cd examples/local-mcp && pip install -r requirements.txt`
- **Swagger**: `swag init -g cmd/uniproxy/main.go`

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
- **PORT**: Server port (default: 8911 for uniproxy service)
- **HOST**: Server host (default: localhost)
- **API_KEY**: API key for authentication
- **SMARTAGENTS_ENABLED**: Enable/disable smartagents service (default: true)
- **REGISTRY_ENABLED**: Enable/disable registry service (default: true)
- **REGISTRY_ENABLE_OFFICIAL**: Enable official registry sources (default: true)

### Service Ports (when running `suse-ai-up all`)
- **Uniproxy**: Port 8911 (HTTP) / 38911 (HTTPS)
- **Registry**: Port 8913 (HTTP) / 38913 (HTTPS)
- **Discovery**: Port 8912 (HTTP) / 38912 (HTTPS)
- **Plugins**: Port 8914 (HTTP) / 38914 (HTTPS)

No Cursor or Copilot rules found in the repository.