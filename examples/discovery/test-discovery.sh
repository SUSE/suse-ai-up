#!/bin/bash

# Simple MCP Discovery Test Script

echo "🧪 Testing MCP Discovery System"
echo

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Function to get host IP
get_host_ip() {
    python3 -c "
import socket
try:
    hostname = socket.gethostname()
    ip_address = socket.gethostbyname(hostname)
    if ip_address.startswith('127.'):
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.connect(('8.8.8.8', 80))
        ip_address = s.getsockname()[0]
        s.close()
    print(ip_address)
except Exception:
    print('127.0.0.1')
"
}

# Get host IP
HOST_IP=$(get_host_ip)
echo -e "${GREEN}📡 Using host IP: $HOST_IP${NC}"
echo

# Check if MCP gateway is running
echo "Checking MCP gateway..."
if curl -s http://localhost:8911/ping >/dev/null 2>&1; then
    echo -e "${GREEN}✅ MCP Gateway is running${NC}"
else
    echo -e "${RED}❌ MCP Gateway not running on port 8911${NC}"
    echo "Please start the MCP gateway first: ./main"
    exit 1
fi

echo
echo "Starting test servers..."

# Start test servers in background
python3 no-auth-server.py >/dev/null 2>&1 &
NO_AUTH_PID=$!

python3 bearer-auth-server.py >/dev/null 2>&1 &
BEARER_PID=$!

python3 oauth-server.py >/dev/null 2>&1 &
OAUTH_PID=$!

# Wait for servers to start
sleep 5

echo "Running discovery scan..."

# Run discovery scan
SCAN_RESPONSE=$(curl -s -X POST http://localhost:8911/scan \
    -H "Content-Type: application/json" \
    -d "{
        \"scanRanges\": [\"$HOST_IP/32\"],
        \"ports\": [8001, 8002, 8004],
        \"timeout\": \"10s\"
    }")

echo "Scan response: $SCAN_RESPONSE"

# Extract scan ID
SCAN_ID=$(echo "$SCAN_RESPONSE" | grep -o '"id":"[^"]*"' | cut -d'"' -f4)

if [ -z "$SCAN_ID" ]; then
    echo -e "${RED}❌ Failed to start scan${NC}"
    kill $NO_AUTH_PID $BEARER_PID $OAUTH_PID 2>/dev/null
    exit 1
fi

echo "Scan ID: $SCAN_ID"

# Wait for scan to complete
echo "Waiting for scan to complete..."
sleep 15

# Get scan results
RESULTS=$(curl -s http://localhost:8911/scan/$SCAN_ID)

echo "Scan results: $RESULTS"

# Check results
SERVER_COUNT=$(echo "$RESULTS" | grep -o '"serverCount":[0-9]*' | cut -d':' -f2)

echo
echo "Results Summary:"
echo "  Servers found: $SERVER_COUNT"

if [ "$SERVER_COUNT" -ge 3 ]; then
    echo -e "${GREEN}✅ Discovery test PASSED - Found expected servers${NC}"
else
    echo -e "${RED}❌ Discovery test FAILED - Expected 3 servers, found $SERVER_COUNT${NC}"
fi

# Cleanup
echo
echo "Cleaning up..."
kill $NO_AUTH_PID $BEARER_PID $OAUTH_PID 2>/dev/null

echo -e "${GREEN}Test completed${NC}"