from fastmcp import FastMCP
from fastmcp.server.auth import StaticTokenVerifier
import os
import signal
import sys

# Transport mode: "stdio" or "http"
transport = os.getenv("MCP_TRANSPORT", "stdio")

# Authorization configuration (only for HTTP)
AUTH_TOKEN = os.getenv("MCP_AUTH_TOKEN", "mcp-example-token-12345")

token_verifier = None
if transport == "http":
    token_verifier = StaticTokenVerifier(
        tokens={
            AUTH_TOKEN: {
                "client_id": "mcp-example-client",
                "scopes": ["read", "write"],
                "expires_at": None  # Never expires
            }
        },
        required_scopes=["read"]
    )

# Create server
server_name = "MCP Example Server with Auth" if transport == "http" else "MCP Example Server"

# Configure FastMCP for HTTP transport with proper security
if transport == "http":
    # Configure trusted proxies and security settings
    import os
    # Set trusted proxies to enable request filtering
    os.environ['FASTMCP_TRUSTED_PROXIES'] = '127.0.0.1,::1,localhost'
    # Enable secure headers
    os.environ['FASTMCP_SECURE_HEADERS'] = 'true'
    # Disable CSRF protection warnings for development
    os.environ['FASTMCP_CSRF_PROTECTION'] = 'false'

app = FastMCP(server_name, auth=token_verifier) if token_verifier else FastMCP(server_name)

@app.tool()
def add(a: int, b: int) -> int:
    """Add two numbers"""
    print(f"[debug-server] add({a}, {b})")
    return a + b

@app.tool()
def multiply(a: int, b: int) -> int:
    """Multiply two numbers"""
    print(f"[debug-server] multiply({a}, {b})")
    return a * b

@app.tool()
def get_server_info() -> dict:
    """Get server information"""
    auth_required = transport == "http"
    return {
        "name": server_name,
        "version": "1.0.0",
        "description": "Example MCP server",
        "auth_required": auth_required,
        "auth_method": "Bearer token" if auth_required else None,
        "token_format": "Authorization: Bearer <token>" if auth_required else None,
        "expected_token": AUTH_TOKEN if auth_required else None,
        "supported_protocols": ["2024-11-05"]
    }

def signal_handler(sig, frame):
    print("Exiting gracefully")
    sys.exit(0)

if __name__ == "__main__":
    signal.signal(signal.SIGTERM, signal_handler)
    signal.signal(signal.SIGINT, signal_handler)
    if transport == "stdio":
        app.run(transport="stdio")
    else:
        app.run(transport="streamable-http", host="127.0.0.1", port=8001)
