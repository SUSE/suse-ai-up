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
    port = int(os.getenv("PORT", "8002"))
    print(f"🚀 Starting No-Auth MCP Server on port {port}")
    print("   This server has NO authentication - high vulnerability")
    print(f"   Test with: curl -X POST http://localhost:{port}/mcp -H 'Content-Type: application/json' -d '{{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{{\"protocolVersion\":\"2024-11-05\",\"capabilities\":{{}},\"clientInfo\":{{\"name\":\"test\",\"version\":\"1.0\"}}}}}}'")
    print()

    app.run(transport="streamable-http", host="127.0.0.1", port=port)