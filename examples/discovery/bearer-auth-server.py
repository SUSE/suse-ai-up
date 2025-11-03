#!/usr/bin/env python3
"""
MCP Discovery Test Server - Bearer Token Authentication
======================================================

This server provides a Bearer token authenticated MCP server for testing
the discovery system's ability to detect servers with Bearer token authentication.

Runs on port 8001 by default.
Uses StaticTokenVerifier for Bearer token validation.
"""

from fastmcp import FastMCP
from fastmcp.server.auth import StaticTokenVerifier
import os
import socket

def get_host_ip():
    """Get the actual IP address of the host"""
    try:
        # Get the hostname
        hostname = socket.gethostname()
        # Get the IP address
        ip_address = socket.gethostbyname(hostname)
        # Validate it's not localhost
        if ip_address.startswith('127.'):
            # Try to get the IP from a socket connection
            s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            s.connect(("8.8.8.8", 80))  # Connect to Google DNS
            ip_address = s.getsockname()[0]
            s.close()
        return ip_address
    except Exception:
        # Fallback to localhost if detection fails
        return "127.0.0.1"

# Bearer token configuration
AUTH_TOKEN = os.getenv("MCP_AUTH_TOKEN", "test-bearer-token-12345")

# Create token verifier for Bearer authentication
token_verifier = StaticTokenVerifier(
    tokens={
        AUTH_TOKEN: {
            "client_id": "mcp-test-client",
            "scopes": ["read", "write"],
            "expires_at": None  # Never expires for testing
        }
    },
    required_scopes=["read"]
)

# Create server with Bearer authentication
app = FastMCP("MCP Server (Bearer Auth)", auth=token_verifier)

@app.tool()
def add(a: int, b: int) -> int:
    """Add two numbers"""
    print(f"[bearer-auth-server] add({a}, {b})")
    return a + b

@app.tool()
def multiply(a: int, b: int) -> int:
    """Multiply two numbers"""
    print(f"[bearer-auth-server] multiply({a}, {b})")
    return a * b

@app.tool()
def get_server_info() -> dict:
    """Get server information"""
    return {
        "name": "MCP Server (Bearer Auth)",
        "version": "1.0.0",
        "description": "Test server with Bearer token authentication",
        "auth_required": True,
        "auth_method": "Bearer token",
        "token_format": "Authorization: Bearer <token>",
        "expected_token": AUTH_TOKEN,
        "supported_protocols": ["2024-11-05"]
    }

@app.tool()
def get_weather(city: str) -> dict:
    """Get weather information for a city (requires auth)"""
    return {
        "city": city,
        "temperature": 22,
        "condition": "sunny",
        "note": "This tool requires Bearer authentication"
    }

if __name__ == "__main__":
    host_ip = get_host_ip()
    port = int(os.getenv("PORT", "8001"))
    print(f"🔐 Starting Bearer Auth MCP Server on {host_ip}:{port}")
    print("   This server requires Bearer token authentication - medium vulnerability")
    print(f"   Test token: {AUTH_TOKEN}")
    print(f"   Accessible at: http://{host_ip}:{port}")
    print(f"   Test with: curl -X POST http://{host_ip}:{port}/mcp -H 'Content-Type: application/json' -H 'Authorization: Bearer {AUTH_TOKEN}' -d '{{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{{\"protocolVersion\":\"2024-11-05\",\"capabilities\":{{}},\"clientInfo\":{{\"name\":\"test\",\"version\":\"1.0\"}}}}}}'")
    print()

    app.run(transport="streamable-http", host=host_ip, port=port)