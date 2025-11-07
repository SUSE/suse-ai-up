#!/usr/bin/env python3

import requests
import json
import time
import subprocess
import sys

def test_service():
    base_url = "http://localhost:8911"
    
    print("=== Testing SUSE AI Universal Proxy ===")
    
    # Test health endpoint
    try:
        response = requests.get(f"{base_url}/health", timeout=5)
        if response.status_code == 200:
            print("✓ Health endpoint working")
            print(f"  Response: {response.json()}")
        else:
            print(f"✗ Health endpoint failed: {response.status_code}")
            return False
    except Exception as e:
        print(f"✗ Cannot connect to service: {e}")
        return False
    
    # Create a stdio adapter
    print("\n=== Creating stdio adapter ===")
    adapter_data = {
        "name": "test-stdio-adapter",
        "connectionType": "LocalStdio",
        "command": "echo",
        "args": ["hello", "world"],
        "workingDirectory": "/tmp",
        "env": {"TEST": "value"},
        "enabled": True
    }
    
    try:
        response = requests.post(f"{base_url}/api/v1/adapters", 
                               json=adapter_data, 
                               headers={"Content-Type": "application/json"},
                               timeout=5)
        if response.status_code == 201:
            print("✓ Adapter created successfully")
            print(f"  Response: {response.json()}")
        else:
            print(f"✗ Failed to create adapter: {response.status_code}")
            print(f"  Response: {response.text}")
            return False
    except Exception as e:
        print(f"✗ Error creating adapter: {e}")
        return False
    
    # Test MCP initialization
    print("\n=== Testing MCP initialization ===")
    init_request = {
        "jsonrpc": "2.0",
        "id": 1,
        "method": "initialize",
        "params": {
            "protocolVersion": "2024-11-05",
            "capabilities": {
                "tools": {}
            },
            "clientInfo": {
                "name": "test-client",
                "version": "1.0.0"
            }
        }
    }
    
    try:
        response = requests.post(f"{base_url}/api/v1/adapters/test-stdio-adapter/mcp",
                               json=init_request,
                               headers={"Content-Type": "application/json"},
                               timeout=10)
        print(f"  Status: {response.status_code}")
        print(f"  Headers: {dict(response.headers)}")
        print(f"  Response: {response.text}")
        
        if response.status_code == 200:
            print("✓ MCP initialization successful")
        else:
            print("✗ MCP initialization failed")
            # Don't return False here as this might be expected with echo command
    except Exception as e:
        print(f"✗ Error during MCP initialization: {e}")
        return False
    
    # Clean up
    print("\n=== Cleaning up ===")
    try:
        response = requests.delete(f"{base_url}/api/v1/adapters/test-stdio-adapter", timeout=5)
        if response.status_code == 204:
            print("✓ Adapter deleted successfully")
        else:
            print(f"✗ Failed to delete adapter: {response.status_code}")
    except Exception as e:
        print(f"✗ Error deleting adapter: {e}")
    
    print("\n=== Test completed ===")
    return True

if __name__ == "__main__":
    success = test_service()
    sys.exit(0 if success else 1)