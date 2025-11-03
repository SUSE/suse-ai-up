#!/usr/bin/env python3
"""
MCP Discovery Test Server - OAuth 2.1 Authentication
===================================================

This server provides an OAuth 2.1 protected MCP server for testing
the discovery system's ability to detect OAuth-authenticated servers.

Features:
- OAuth 2.0 Authorization Server (port 8003)
- OAuth 2.1 Protected MCP Server (port 8004)
- Resource metadata discovery
- Proper WWW-Authenticate headers with resource_metadata

Architecture:
- Port 8003: OAuth authorization/token server
- Port 8004: MCP server protected by OAuth
"""

from fastmcp import FastMCP
from fastmcp.server.auth import StaticTokenVerifier
import os
import socket
import threading
import time
from flask import Flask, jsonify, request, redirect
from flask_cors import CORS

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

# OAuth Configuration
OAUTH_PORT = int(os.getenv("OAUTH_PORT", "8003"))
MCP_PORT = int(os.getenv("MCP_PORT", "8004"))
CLIENT_ID = "mcp-oauth-test-client"
CLIENT_SECRET = "mcp-oauth-test-secret"

# Flask OAuth Server (simplified for testing)
oauth_app = Flask(__name__)
CORS(oauth_app)

# In-memory storage for OAuth (for testing only)
tokens = {}
auth_codes = {}

# OAuth Routes
@oauth_app.route('/.well-known/oauth-protected-resource', methods=['GET'])
def protected_resource_metadata():
    """OAuth 2.1 protected resource metadata"""
    host_ip = get_host_ip()
    return jsonify({
        "resource": f"http://{host_ip}:{MCP_PORT}",
        "authorization_servers": [f"http://{host_ip}:{OAUTH_PORT}"],
        "scopes": ["read", "write", "mcp:tools"],
        "resource_documentation": f"http://{host_ip}:{OAUTH_PORT}/docs"
    })

@oauth_app.route('/.well-known/oauth-authorization-server', methods=['GET'])
def authorization_server_metadata():
    """OAuth 2.0 authorization server metadata"""
    host_ip = get_host_ip()
    return jsonify({
        "issuer": f"http://{host_ip}:{OAUTH_PORT}",
        "authorization_endpoint": f"http://{host_ip}:{OAUTH_PORT}/oauth/authorize",
        "token_endpoint": f"http://{host_ip}:{OAUTH_PORT}/oauth/token",
        "jwks_uri": f"http://{host_ip}:{OAUTH_PORT}/.well-known/jwks.json",
        "scopes_supported": ["read", "write", "mcp:tools"],
        "response_types_supported": ["code"],
        "grant_types_supported": ["authorization_code", "refresh_token"],
        "token_endpoint_auth_methods_supported": ["client_secret_basic"]
    })

@oauth_app.route('/oauth/authorize', methods=['GET'])
def authorize():
    """OAuth authorization endpoint"""
    client_id = request.args.get('client_id')
    redirect_uri = request.args.get('redirect_uri')
    scope = request.args.get('scope', 'read')
    state = request.args.get('state', '')

    if client_id != CLIENT_ID:
        return "Invalid client", 400

    # Generate authorization code
    auth_code = f"auth_code_{int(time.time())}"
    auth_codes[auth_code] = {
        'client_id': client_id,
        'redirect_uri': redirect_uri,
        'scope': scope,
        'state': state
    }

    # Auto-approve for testing
    redirect_url = f"{redirect_uri}?code={auth_code}&state={state}"
    return redirect(redirect_url)

@oauth_app.route('/oauth/token', methods=['POST'])
def token():
    """OAuth token endpoint"""
    auth = request.authorization
    if not auth or auth.username != CLIENT_ID or auth.password != CLIENT_SECRET:
        return jsonify({"error": "invalid_client"}), 401

    grant_type = request.form.get('grant_type')
    code = request.form.get('code')

    if grant_type != 'authorization_code' or code not in auth_codes:
        return jsonify({"error": "invalid_grant"}), 400

    # Generate access token
    access_token = f"access_token_{int(time.time())}"
    tokens[access_token] = auth_codes[code]

    return jsonify({
        "access_token": access_token,
        "token_type": "Bearer",
        "expires_in": 3600,
        "scope": auth_codes[code]['scope']
    })

# MCP Server with simplified OAuth-style protection
# For testing purposes, we'll use StaticTokenVerifier but return OAuth-style headers
token_verifier = StaticTokenVerifier(
    tokens={
        "oauth-test-token": {
            "client_id": "mcp-oauth-test-client",
            "scopes": ["read", "write", "mcp:tools"],
            "expires_at": None  # Never expires for testing
        }
    },
    required_scopes=["read"]
)

app = FastMCP("MCP OAuth Protected Server", auth=token_verifier)

@app.tool()
def add(a: int, b: int) -> int:
    """Add two numbers (OAuth protected)"""
    print(f"[oauth-mcp-server] add({a}, {b})")
    return a + b

@app.tool()
def multiply(a: int, b: int) -> int:
    """Multiply two numbers (OAuth protected)"""
    print(f"[oauth-mcp-server] multiply({a}, {b})")
    return a * b

@app.tool()
def get_server_info() -> dict:
    """Get server information"""
    host_ip = get_host_ip()
    return {
        "name": "MCP OAuth Protected Server",
        "version": "1.0.0",
        "description": "Test server with OAuth 2.1 authentication (simplified for discovery testing)",
        "auth_required": True,
        "auth_method": "OAuth 2.1",
        "oauth_metadata": f"http://{host_ip}:{OAUTH_PORT}/.well-known/oauth-protected-resource",
        "test_token": "oauth-test-token",
        "supported_protocols": ["2024-11-05"]
    }

@app.tool()
def get_protected_data() -> dict:
    """Get protected data (requires OAuth)"""
    return {
        "secret": "This data is protected by OAuth 2.1",
        "timestamp": int(time.time()),
        "access_level": "authenticated"
    }

def run_oauth_server():
    """Run the OAuth server"""
    host_ip = get_host_ip()
    print(f"🔐 Starting OAuth Server on {host_ip}:{OAUTH_PORT}")
    oauth_app.run(host=host_ip, port=OAUTH_PORT, debug=False)

if __name__ == "__main__":
    host_ip = get_host_ip()
    print("🚀 Starting OAuth 2.1 MCP Test Environment")
    print(f"   OAuth Server: http://{host_ip}:{OAUTH_PORT}")
    print(f"   MCP Server: http://{host_ip}:{MCP_PORT}")
    print("   This setup provides OAuth 2.1 protection - low vulnerability")
    print()
    print("Testing OAuth Detection:")
    print("1. Discovery will detect OAuth metadata endpoints")
    print("2. Auth type will be classified as 'oauth' with 'low' vulnerability")
    print("3. To test manually:")
    print(f"   curl -I http://{host_ip}:{MCP_PORT}/mcp")
    print("   (Should return 401 with WWW-Authenticate header)")
    print(f"   curl -H 'Authorization: Bearer oauth-test-token' http://{host_ip}:{MCP_PORT}/mcp")
    print("   (Should work with valid token)")
    print()

    # Start OAuth server in background
    oauth_thread = threading.Thread(target=run_oauth_server, daemon=True)
    oauth_thread.start()

    # Give OAuth server time to start
    time.sleep(2)

    # Start MCP server
    print(f"🛡️ Starting OAuth-protected MCP Server on {host_ip}:{MCP_PORT}")
    app.run(transport="streamable-http", host=host_ip, port=MCP_PORT)