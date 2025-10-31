#!/bin/bash

echo "=== MCP Server Discovery Workflow ==="
echo

echo "Step 1: Scanning for MCP servers on 192.168.1.74 ports 8000 and 8001..."
SCAN_RESPONSE=$(curl -s -X POST http://localhost:8911/scan \
  -H "Content-Type: application/json" \
  -d '{
    "scanRanges": ["192.168.1.74/32"],
    "ports": [8000, 8001]
  }')

echo "Scan Response:"
echo "$SCAN_RESPONSE" | jq '.'
echo

# Extract scan ID and server count
SCAN_ID=$(echo "$SCAN_RESPONSE" | jq -r '.scanId')
SERVER_COUNT=$(echo "$SCAN_RESPONSE" | jq -r '.serverCount')

echo "Step 2: Listing discovered servers..."
SERVERS_RESPONSE=$(curl -s http://localhost:8911/servers)
echo "Discovered Servers:"
echo "$SERVERS_RESPONSE" | jq '.'
echo

if [ "$SERVER_COUNT" -gt 0 ]; then
    # Get the first server ID
    SERVER_ID=$(echo "$SERVERS_RESPONSE" | jq -r '.[0].id')
    SERVER_ADDRESS=$(echo "$SERVERS_RESPONSE" | jq -r '.[0].address')
    
    echo "Step 3: Registering discovered server (ID: $SERVER_ID)..."
    REGISTER_RESPONSE=$(curl -s -X POST http://localhost:8911/register \
      -H "Content-Type: application/json" \
      -d "{\"discoveredServerId\": \"$SERVER_ID\"}")
    
    echo "Registration Response:"
    echo "$REGISTER_RESPONSE" | jq '.'
    echo
    
    echo "Step 4: Creating adapter for discovered server..."
    ADAPTER_RESPONSE=$(curl -s -X POST http://localhost:8911/adapters \
      -H "Content-Type: application/json" \
      -d "{
        \"name\": \"discovered-mcp-server-192-168-1-74\",
        \"imageName\": \"mcp-proxy\",
        \"imageVersion\": \"1.0.0\",
        \"protocol\": \"MCP\",
        \"connectionType\": \"SSE\",
        \"environmentVariables\": {
          \"MCP_PROXY_URL\": \"$SERVER_ADDRESS/mcp\"
        },
        \"description\": \"Auto-discovered MCP server on $SERVER_ADDRESS\"
      }")
    
    echo "Adapter Creation Response:"
    echo "$ADAPTER_RESPONSE" | jq '.'
    echo
    
    echo "Step 5: Testing SSE connection..."
    echo "SSE Connection (will show ping messages):"
    timeout 5 curl -s -N -H "Accept: text/event-stream" http://localhost:8911/adapters/discovered-mcp-server-192-168-1-74/sse || echo "SSE test completed"
    echo
    
    echo "Step 6: Testing Streamable HTTP connection..."
    MCP_RESPONSE=$(curl -s -X POST http://localhost:8911/adapters/discovered-mcp-server-192-168-1-74/mcp \
      -H "Content-Type: application/json" \
      -H "Accept: application/json, text/event-stream" \
      -d '{
        "jsonrpc": "2.0",
        "id": 1,
        "method": "initialize",
        "params": {
          "protocolVersion": "2024-11-05",
          "capabilities": {},
          "clientInfo": {
            "name": "test-client",
            "version": "1.0"
          }
        }
      }')
    
    echo "MCP Initialize Response:"
    echo "$MCP_RESPONSE" | head -5
    echo
    
else
    echo "No MCP servers found during scan."
fi

echo "=== Discovery Workflow Complete ==="
