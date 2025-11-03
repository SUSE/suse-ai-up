#!/usr/bin/env python3
"""
MCP Discovery Test Server - No Authentication
===========================================

This server provides an unauthenticated MCP server for testing
the discovery system's ability to detect servers without authentication.

Runs on port 8002 by default.
"""

from fastmcp import FastMCP
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

# Create server without authentication
app = FastMCP("MCP Example Server (No Auth)")

@app.tool()
def add(a: int, b: int) -> int:
    """Add two numbers"""
    print(f"[no-auth-server] add({a}, {b})")
    return a + b

@app.tool()
def multiply(a: int, b: int) -> int:
    """Multiply two numbers"""
    print(f"[no-auth-server] multiply({a}, {b})")
    return a * b

@app.tool()
def get_server_info() -> dict:
    """Get server information"""
    return {
        "name": "MCP Example Server (No Auth)",
        "version": "1.0.0",
        "description": "Test server without authentication",
        "auth_required": False,
        "supported_protocols": ["2024-11-05"]
    }

if __name__ == "__main__":
    host_ip = get_host_ip()
    port = int(os.getenv("PORT", "8002"))
    print(f"🚀 Starting No-Auth MCP Server on {host_ip}:{port}")
    print("   This server has NO authentication - high vulnerability")
    print(f"   Accessible at: http://{host_ip}:{port}")
    print(f"   Test with: curl -X POST http://{host_ip}:{port}/mcp -H 'Content-Type: application/json' -d '{{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{{\"protocolVersion\":\"2024-11-05\",\"capabilities\":{{}},\"clientInfo\":{{\"name\":\"test\",\"version\":\"1.0\"}}}}}}'")
    print()

    app.run(transport="streamable-http", host=host_ip, port=port)