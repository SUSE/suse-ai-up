#!/usr/bin/env python3

import requests
import json
import time
import subprocess
import sys
import os

def start_mcp_server():
    """Start the example MCP server in background"""
    print("Starting MCP stdio server...")
    env = os.environ.copy()
    env['MCP_TRANSPORT'] = 'stdio'
    
    proc = subprocess.Popen([
        'python3', 'examples/proxy/src/main.py'
    ], env=env, cwd='.')
    return proc

def test_service():
    base_url = "http://localhost:8911"
    
    print("=== Testing SUSE AI Universal Proxy with Real MCP Server ===")
    
    # Test health endpoint
    try:
        response = requests.get(f"{base_url}/health", timeout=5)
        if response.status_code == 200:
            print("✓ Health endpoint working")
        else:
            print(f"✗ Health endpoint failed: {response.status_code}")
            return False
    except Exception as e:
        print(f"✗ Cannot connect to service: {e}")
        return False
    
    # Create a stdio adapter for the real MCP server
    print("\n=== Creating stdio adapter for real MCP server ===")
    adapter_data = {
        "name": "real-mcp-adapter",
        "connectionType": "LocalStdio",
        "command": "python3",
        "args": ["examples/proxy/src/main.py"],
        "workingDirectory": ".",
        "env": {"MCP_TRANSPORT": "stdio"},
        "enabled": True
    }
    
    try:
        response = requests.post(f"{base_url}/api/v1/adapters", 
                               json=adapter_data, 
                               headers={"Content-Type": "application/json"},
                               timeout=5)
        if response.status_code == 201:
            print("✓ Adapter created successfully")
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
        response = requests.post(f"{base_url}/api/v1/adapters/real-mcp-adapter/mcp",
                               json=init_request,
                               headers={"Content-Type": "application/json"},
                               timeout=15)
        print(f"  Status: {response.status_code}")
        
        if response.status_code == 200:
            print("✓ MCP initialization successful")
            init_response = response.json()
            session_id = response.headers.get('Mcp-Session-Id')
            print(f"  Session ID: {session_id}")
            
            # Test tools/list
            if session_id:
                print("\n=== Testing tools/list ===")
                tools_request = {
                    "jsonrpc": "2.0",
                    "id": 2,
                    "method": "tools/list"
                }
                
                response = requests.post(f"{base_url}/api/v1/adapters/real-mcp-adapter/mcp",
                                       json=tools_request,
                                       headers={
                                           "Content-Type": "application/json",
                                           "Mcp-Session-Id": session_id
                                       },
                                       timeout=10)
                
                print(f"  Status: {response.status_code}")
                if response.status_code == 200:
                    tools_response = response.json()
                    print("✓ tools/list successful")
                    if 'result' in tools_response and 'tools' in tools_response['result']:
                        tools = tools_response['result']['tools']
                        print(f"  Available tools: {[tool.get('name', 'unknown') for tool in tools]}")
                        
                        # Test calling a tool
                        if tools:
                            print("\n=== Testing tools/call ===")
                            tool_name = tools[0].get('name')
                            if tool_name:
                                call_request = {
                                    "jsonrpc": "2.0",
                                    "id": 3,
                                    "method": "tools/call",
                                    "params": {
                                        "name": tool_name,
                                        "arguments": {"a": 5, "b": 3} if tool_name in ['add', 'multiply'] else {}
                                    }
                                }
                                
                                response = requests.post(f"{base_url}/api/v1/adapters/real-mcp-adapter/mcp",
                                                       json=call_request,
                                                       headers={
                                                           "Content-Type": "application/json",
                                                           "Mcp-Session-Id": session_id
                                                       },
                                                       timeout=10)
                                
                                print(f"  Status: {response.status_code}")
                                if response.status_code == 200:
                                    print("✓ tools/call successful")
                                    call_response = response.json()
                                    if 'result' in call_response:
                                        print(f"  Result: {call_response['result']}")
                                else:
                                    print("✗ tools/call failed")
                                    print(f"  Response: {response.text}")
                else:
                    print("✗ tools/list failed")
                    print(f"  Response: {response.text}")
            else:
                print("✗ No session ID in response")
        else:
            print("✗ MCP initialization failed")
            print(f"  Response: {response.text}")
    except Exception as e:
        print(f"✗ Error during MCP operations: {e}")
        return False
    
    # Clean up
    print("\n=== Cleaning up ===")
    try:
        response = requests.delete(f"{base_url}/api/v1/adapters/real-mcp-adapter", timeout=5)
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