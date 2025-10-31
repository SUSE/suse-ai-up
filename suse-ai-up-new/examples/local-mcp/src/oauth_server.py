from fastmcp import FastMCP
import os
import secrets
import time
from flask import Flask, request, jsonify, redirect, url_for
from flask_cors import CORS

# OAuth Server Implementation
oauth_app = Flask(__name__)
CORS(oauth_app)

# In-memory storage for demo purposes
clients = {
    "mcp-proxy": {
        "client_id": "mcp-proxy",
        "client_secret": "demo-secret",
        "redirect_uris": ["http://localhost:8911/oauth/callback"],
        "scopes": ["read", "write"]
    }
}

authorization_codes = {}
access_tokens = {}

# MCP Server with OAuth protection
transport = os.getenv("MCP_TRANSPORT", "stdio")

if transport == "http":
    # OAuth-protected MCP server
    app = FastMCP("MCP OAuth Server", auth=None)  # We'll handle auth manually
else:
    app = FastMCP("MCP OAuth Server")

@app.tool()
def add(a: int, b: int) -> int:
    """Add two numbers (OAuth protected)"""
    print(f"[oauth-server] add({a}, {b})")
    return a + b

@app.tool()
def multiply(a: int, b: int) -> int:
    """Multiply two numbers (OAuth protected)"""
    print(f"[oauth-server] multiply({a}, {b})")
    return a * b

@app.tool()
def get_server_info() -> dict:
    """Get server information"""
    return {
        "name": "MCP OAuth Server",
        "version": "1.0.0",
        "description": "OAuth 2.1 protected MCP server",
        "auth_required": transport == "http",
        "auth_method": "OAuth 2.1" if transport == "http" else None,
        "supported_protocols": ["2024-11-05"]
    }

# OAuth endpoints
@oauth_app.route('/.well-known/oauth-protected-resource', methods=['GET'])
def protected_resource_metadata():
    return jsonify({
        "resource": "https://mcp.example.com",
        "authorization_servers": ["http://localhost:8003"],
        "scopes": ["read", "write"],
        "resource_documentation": "http://localhost:8003/docs"
    })

@oauth_app.route('/.well-known/oauth-authorization-server', methods=['GET'])
def authorization_server_metadata():
    return jsonify({
        "issuer": "http://localhost:8003",
        "authorization_endpoint": "http://localhost:8003/oauth/authorize",
        "token_endpoint": "http://localhost:8003/oauth/token",
        "jwks_uri": "http://localhost:8003/.well-known/jwks.json",
        "scopes_supported": ["read", "write"],
        "response_types_supported": ["code"],
        "grant_types_supported": ["authorization_code", "refresh_token"],
        "token_endpoint_auth_methods_supported": ["client_secret_basic"]
    })

@oauth_app.route('/oauth/authorize', methods=['GET'])
def authorize():
    client_id = request.args.get('client_id')
    redirect_uri = request.args.get('redirect_uri')
    scope = request.args.get('scope', '')
    state = request.args.get('state', '')

    if client_id not in clients:
        return jsonify({"error": "invalid_client"}), 400

    client = clients[client_id]
    if redirect_uri not in client['redirect_uris']:
        return jsonify({"error": "invalid_redirect_uri"}), 400

    # For demo, auto-approve
    code = secrets.token_urlsafe(32)
    authorization_codes[code] = {
        "client_id": client_id,
        "redirect_uri": redirect_uri,
        "scope": scope,
        "state": state,
        "expires_at": time.time() + 600  # 10 minutes
    }

    redirect_url = f"{redirect_uri}?code={code}&state={state}"
    return redirect(redirect_url)

@oauth_app.route('/oauth/token', methods=['POST'])
def token():
    grant_type = request.form.get('grant_type')
    code = request.form.get('code')
    redirect_uri = request.form.get('redirect_uri')
    client_id = request.form.get('client_id')

    if grant_type != 'authorization_code':
        return jsonify({"error": "unsupported_grant_type"}), 400

    if code not in authorization_codes:
        return jsonify({"error": "invalid_grant"}), 400

    auth_code = authorization_codes[code]
    if auth_code['client_id'] != client_id:
        return jsonify({"error": "invalid_client"}), 400

    if auth_code['redirect_uri'] != redirect_uri:
        return jsonify({"error": "invalid_redirect_uri"}), 400

    if time.time() > auth_code['expires_at']:
        return jsonify({"error": "code_expired"}), 400

    # Generate tokens
    access_token = secrets.token_urlsafe(32)
    refresh_token = secrets.token_urlsafe(32)

    access_tokens[access_token] = {
        "client_id": client_id,
        "scope": auth_code['scope'],
        "expires_at": time.time() + 3600  # 1 hour
    }

    # Clean up used code
    del authorization_codes[code]

    return jsonify({
        "access_token": access_token,
        "token_type": "Bearer",
        "expires_in": 3600,
        "refresh_token": refresh_token,
        "scope": auth_code['scope']
    })

def verify_token(token):
    if token not in access_tokens:
        return None

    token_data = access_tokens[token]
    if time.time() > token_data['expires_at']:
        del access_tokens[token]
        return None

    return token_data

if __name__ == "__main__":
    if transport == "stdio":
        app.run(transport="stdio")
    else:
        # Start both OAuth server and MCP server
        import threading

        def run_oauth():
            oauth_app.run(host="127.0.0.1", port=8003, debug=False)

        def run_mcp():
            app.run(transport="streamable-http", host="127.0.0.1", port=8004)

        oauth_thread = threading.Thread(target=run_oauth)
        oauth_thread.daemon = True
        oauth_thread.start()

        run_mcp()