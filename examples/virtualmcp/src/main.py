#!/usr/bin/env python3
"""
VirtualMCP Service - Provides MCP implementation discovery and management
"""

import json
import logging
import os
import sys
import time
from datetime import datetime
from typing import Dict, List, Optional, Any
from uuid import uuid4

import requests
from flask import Flask, request, jsonify
from flask_cors import CORS

# Configure logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(name)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)

app = Flask(__name__)
CORS(app)

# Configuration
PORT = int(os.getenv("VIRTUALMCP_PORT", "8913"))
PROXY_URL = os.getenv("PROXY_URL", "http://localhost:8911")
SERVICE_ID = os.getenv("SERVICE_ID", f"virtualmcp-{uuid4().hex[:8]}")

class MCPImplementation:
    """Represents an MCP implementation"""

    def __init__(self, id: str, name: str, description: str, version: str = "1.0.0"):
        self.id = id
        self.name = name
        self.description = description
        self.version = version
        self.transport = "stdio"  # Default transport
        self.capabilities: List[str] = []
        self.config_template: Dict[str, Any] = {}
        self.tools: List[Dict[str, Any]] = []
        self.prompts: List[Dict[str, Any]] = []
        self.created_at = datetime.utcnow().isoformat()
        self.updated_at = self.created_at

    def to_dict(self) -> Dict[str, Any]:
        return {
            "id": self.id,
            "name": self.name,
            "description": self.description,
            "version": self.version,
            "transport": self.transport,
            "capabilities": self.capabilities,
            "config_template": self.config_template,
            "tools": self.tools,
            "prompts": self.prompts,
            "created_at": self.created_at,
            "updated_at": self.updated_at,
        }

class MCPRegistry:
    """Registry for MCP implementations"""

    def __init__(self):
        self.implementations: Dict[str, MCPImplementation] = {}
        self._initialize_default_implementations()

    def _initialize_default_implementations(self):
        """Initialize with some default MCP implementations"""

        # Anthropic Claude implementation
        claude = MCPImplementation(
            id="anthropic-claude-3",
            name="Anthropic Claude 3",
            description="Claude 3 model via Anthropic API",
            version="1.0.0"
        )
        claude.capabilities = ["chat", "completion", "tools"]
        claude.config_template = {
            "command": "python",
            "args": ["claude_server.py"],
            "env": {
                "ANTHROPIC_API_KEY": ""
            },
            "transport": "stdio"
        }
        claude.tools = [
            {
                "name": "chat_completion",
                "description": "Generate chat completions using Claude 3",
                "input_schema": {
                    "type": "object",
                    "properties": {
                        "messages": {
                            "type": "array",
                            "description": "Chat messages"
                        },
                        "max_tokens": {
                            "type": "integer",
                            "description": "Maximum tokens to generate"
                        }
                    },
                    "required": ["messages"]
                }
            }
        ]
        self.implementations[claude.id] = claude

        # OpenAI GPT implementation
        gpt = MCPImplementation(
            id="openai-gpt-4",
            name="OpenAI GPT-4",
            description="GPT-4 model via OpenAI API",
            version="1.0.0"
        )
        gpt.capabilities = ["chat", "completion", "tools", "embeddings"]
        gpt.config_template = {
            "command": "python",
            "args": ["openai_server.py"],
            "env": {
                "OPENAI_API_KEY": ""
            },
            "transport": "stdio"
        }
        gpt.tools = [
            {
                "name": "chat_completion",
                "description": "Generate chat completions using GPT-4",
                "input_schema": {
                    "type": "object",
                    "properties": {
                        "messages": {
                            "type": "array",
                            "description": "Chat messages"
                        },
                        "max_tokens": {
                            "type": "integer",
                            "description": "Maximum tokens to generate"
                        }
                    },
                    "required": ["messages"]
                }
            }
        ]
        self.implementations[gpt.id] = gpt

        # Local filesystem implementation
        filesystem = MCPImplementation(
            id="filesystem-local",
            name="Local Filesystem",
            description="Access local filesystem operations",
            version="1.0.0"
        )
        filesystem.capabilities = ["filesystem", "tools"]
        filesystem.config_template = {
            "command": "python",
            "args": ["filesystem_server.py"],
            "env": {},
            "transport": "stdio"
        }
        filesystem.tools = [
            {
                "name": "read_file",
                "description": "Read contents of a file",
                "input_schema": {
                    "type": "object",
                    "properties": {
                        "path": {
                            "type": "string",
                            "description": "File path to read"
                        }
                    },
                    "required": ["path"]
                }
            },
            {
                "name": "list_directory",
                "description": "List contents of a directory",
                "input_schema": {
                    "type": "object",
                    "properties": {
                        "path": {
                            "type": "string",
                            "description": "Directory path to list"
                        }
                    },
                    "required": ["path"]
                }
            }
        ]
        self.implementations[filesystem.id] = filesystem

        logger.info(f"Initialized {len(self.implementations)} default MCP implementations")

    def get_all_implementations(self) -> List[Dict[str, Any]]:
        """Get all MCP implementations"""
        return [impl.to_dict() for impl in self.implementations.values()]

    def get_implementation(self, impl_id: str) -> Optional[Dict[str, Any]]:
        """Get a specific MCP implementation"""
        impl = self.implementations.get(impl_id)
        return impl.to_dict() if impl else None

    def add_implementation(self, impl_data: Dict[str, Any]) -> Dict[str, Any]:
        """Add a new MCP implementation"""
        impl_id = impl_data.get("id", str(uuid4()))

        impl = MCPImplementation(
            id=impl_id,
            name=impl_data["name"],
            description=impl_data["description"],
            version=impl_data.get("version", "1.0.0")
        )

        impl.transport = impl_data.get("transport", "stdio")
        impl.capabilities = impl_data.get("capabilities", [])
        impl.config_template = impl_data.get("config_template", {})
        impl.tools = impl_data.get("tools", [])
        impl.prompts = impl_data.get("prompts", [])

        self.implementations[impl_id] = impl
        logger.info(f"Added MCP implementation: {impl_id}")
        return impl.to_dict()

    def update_implementation(self, impl_id: str, impl_data: Dict[str, Any]) -> Optional[Dict[str, Any]]:
        """Update an existing MCP implementation"""
        impl = self.implementations.get(impl_id)
        if not impl:
            return None

        # Update fields
        for key, value in impl_data.items():
            if key == "name":
                impl.name = value
            elif key == "description":
                impl.description = value
            elif key == "version":
                impl.version = value
            elif key == "transport":
                impl.transport = value
            elif key == "capabilities":
                impl.capabilities = value
            elif key == "config_template":
                impl.config_template = value
            elif key == "tools":
                impl.tools = value
            elif key == "prompts":
                impl.prompts = value

        impl.updated_at = datetime.utcnow().isoformat()
        logger.info(f"Updated MCP implementation: {impl_id}")
        return impl.to_dict()

    def delete_implementation(self, impl_id: str) -> bool:
        """Delete an MCP implementation"""
        if impl_id in self.implementations:
            del self.implementations[impl_id]
            logger.info(f"Deleted MCP implementation: {impl_id}")
            return True
        return False

# Global registry instance
registry = MCPRegistry()

# API Routes

@app.route("/health", methods=["GET"])
def health_check():
    """Health check endpoint"""
    return jsonify({
        "status": "healthy",
        "service": "virtualmcp",
        "implementations_count": len(registry.implementations),
        "timestamp": datetime.utcnow().isoformat()
    })

@app.route("/api/v1/mcps", methods=["GET"])
def get_mcp_implementations():
    """Get all MCP implementations"""
    try:
        implementations = registry.get_all_implementations()
        return jsonify({
            "implementations": implementations,
            "count": len(implementations),
            "service": SERVICE_ID
        })
    except Exception as e:
        logger.error(f"Error getting MCP implementations: {e}")
        return jsonify({"error": "Internal server error"}), 500

@app.route("/api/v1/mcps/<impl_id>", methods=["GET"])
def get_mcp_implementation(impl_id: str):
    """Get a specific MCP implementation"""
    try:
        impl = registry.get_implementation(impl_id)
        if not impl:
            return jsonify({"error": "Implementation not found"}), 404
        return jsonify(impl)
    except Exception as e:
        logger.error(f"Error getting MCP implementation {impl_id}: {e}")
        return jsonify({"error": "Internal server error"}), 500

@app.route("/api/v1/mcps", methods=["POST"])
def add_mcp_implementation():
    """Add a new MCP implementation"""
    try:
        data = request.get_json()
        if not data:
            return jsonify({"error": "No JSON data provided"}), 400

        required_fields = ["name", "description"]
        for field in required_fields:
            if field not in data:
                return jsonify({"error": f"Missing required field: {field}"}), 400

        impl = registry.add_implementation(data)
        return jsonify(impl), 201
    except Exception as e:
        logger.error(f"Error adding MCP implementation: {e}")
        return jsonify({"error": "Internal server error"}), 500

@app.route("/api/v1/mcps/<impl_id>", methods=["PUT"])
def update_mcp_implementation(impl_id: str):
    """Update an existing MCP implementation"""
    try:
        data = request.get_json()
        if not data:
            return jsonify({"error": "No JSON data provided"}), 400

        impl = registry.update_implementation(impl_id, data)
        if not impl:
            return jsonify({"error": "Implementation not found"}), 404
        return jsonify(impl)
    except Exception as e:
        logger.error(f"Error updating MCP implementation {impl_id}: {e}")
        return jsonify({"error": "Internal server error"}), 500

@app.route("/api/v1/mcps/<impl_id>", methods=["DELETE"])
def delete_mcp_implementation(impl_id: str):
    """Delete an MCP implementation"""
    try:
        if registry.delete_implementation(impl_id):
            return jsonify({"message": "Implementation deleted successfully"})
        else:
            return jsonify({"error": "Implementation not found"}), 404
    except Exception as e:
        logger.error(f"Error deleting MCP implementation {impl_id}: {e}")
        return jsonify({"error": "Internal server error"}), 500

def register_with_proxy():
    """Register this service with the proxy"""
    try:
        registration_data = {
            "service_id": SERVICE_ID,
            "service_type": "virtualmcp",
            "service_url": f"http://localhost:{PORT}",
            "version": "1.0.0",
            "capabilities": [
                {
                    "path": "/api/v1/mcps",
                    "methods": ["GET"],
                    "description": "MCP implementation discovery"
                },
                {
                    "path": "/api/v1/mcps/*",
                    "methods": ["GET", "POST", "PUT", "DELETE"],
                    "description": "MCP implementation management"
                }
            ]
        }

        url = f"{PROXY_URL}/api/v1/plugins/register"
        logger.info(f"Registering with proxy at: {url}")

        response = requests.post(url, json=registration_data, timeout=10)

        if response.status_code == 201:
            logger.info("Successfully registered with proxy")
            return True
        else:
            logger.error(f"Failed to register with proxy: {response.status_code} - {response.text}")
            return False

    except Exception as e:
        logger.error(f"Error registering with proxy: {e}")
        return False

def main():
    """Main entry point"""
    logger.info(f"Starting VirtualMCP service on port {PORT}")
    logger.info(f"Service ID: {SERVICE_ID}")
    logger.info(f"Proxy URL: {PROXY_URL}")

    # Register with proxy
    if not register_with_proxy():
        logger.warning("Failed to register with proxy, but continuing to start service")

    # Start the Flask app
    app.run(host="0.0.0.0", port=PORT, debug=False)

if __name__ == "__main__":
    main()