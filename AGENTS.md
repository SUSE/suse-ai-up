# AGENTS.md

## Build Commands
- **Go**: `cd mcp-gateway && go build ./cmd/service`
- **Python**: `cd mcp-example-server && pip install -r requirements.txt`
- **Swagger**: `cd mcp-gateway && swag init -g cmd/service/main.go`

## Test Commands
- **Go**: `cd mcp-gateway && go test ./...`

## Lint Commands
- **Go**: `gofmt -d .` (check formatting)
- **Python**: No specific linter configured

## Code Style Guidelines
- **Go**: Standard Go conventions, descriptive names, proper error handling
- **Python**: PEP 8, descriptive names, simple and clean code
- **General**: Use dependency injection, avoid abbreviations, add comments for complex logic

## Environment Variables
- **AUTH_MODE**: Set to "oauth" for OAuth authentication, "dev" (default) for development mode
- **PORT**: Server port (default: 8911)

No Cursor or Copilot rules found in the repository.