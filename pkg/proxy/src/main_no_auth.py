from fastmcp import FastMCP
import os
import signal
import sys

# Transport mode: "stdio" or "http"
transport = os.getenv("MCP_TRANSPORT", "stdio")

# Create server (no authentication)
server_name = "MCP Example Server (No Auth)"
app = FastMCP(server_name)

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
    return {
        "name": server_name,
        "version": "1.0.0",
        "description": "Example MCP server without authentication",
        "auth_required": False,
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
        app.run(transport="streamable-http", host="127.0.0.1", port=8002)