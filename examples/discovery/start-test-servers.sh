#!/bin/bash

# Helper script to start all MCP discovery test servers
# This script starts the test servers in the background for manual testing

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}🚀 Starting MCP Discovery Test Servers${NC}"
echo

# Function to check if port is available
check_port() {
    local port=$1
    if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null 2>&1; then
        echo -e "${YELLOW}⚠️  Port $port is already in use${NC}"
        return 1
    else
        return 0
    fi
}

# Function to wait for server to be ready
wait_for_server() {
    local port=$1
    local name=$2
    local count=0
    local max_wait=10

    echo -n "Waiting for $name on port $port..."
    while ! curl -s --max-time 1 "http://localhost:$port/mcp" >/dev/null 2>&1; do
        sleep 1
        ((count++))
        echo -n "."
        if [ $count -gt $max_wait ]; then
            echo -e "\n${RED}❌ $name failed to start within $max_wait seconds${NC}"
            return 1
        fi
    done
    echo -e "\n${GREEN}✅ $name is ready on port $port${NC}"
    return 0
}

# Start No-Auth Server (port 8002)
if check_port 8002; then
    echo "Starting No-Auth Server on port 8002..."
    python3 no-auth-server.py >/dev/null 2>&1 &
    NO_AUTH_PID=$!
    sleep 2
    wait_for_server 8002 "No-Auth Server"
else
    echo "Skipping No-Auth Server (port 8002 in use)"
fi

# Start Bearer Auth Server (port 8001)
if check_port 8001; then
    echo "Starting Bearer Auth Server on port 8001..."
    python3 bearer-auth-server.py >/dev/null 2>&1 &
    BEARER_PID=$!
    sleep 2
    wait_for_server 8001 "Bearer Auth Server"
else
    echo "Skipping Bearer Auth Server (port 8001 in use)"
fi

# Start OAuth Server (ports 8003/8004)
if check_port 8004 && check_port 8003; then
    echo "Starting OAuth Server (OAuth:8003, MCP:8004)..."
    python3 oauth-server.py >/dev/null 2>&1 &
    OAUTH_PID=$!
    sleep 3  # OAuth server takes longer to start
    wait_for_server 8004 "OAuth MCP Server"
else
    echo "Skipping OAuth Server (ports 8003/8004 in use)"
fi

echo
echo -e "${GREEN}🎉 Test servers started successfully!${NC}"
echo
echo "Active servers:"
echo "  📡 No-Auth Server:    http://localhost:8002 (No authentication)"
echo "  🔐 Bearer Auth Server: http://localhost:8001 (Bearer token required)"
echo "  🛡️  OAuth Server:     http://localhost:8004 (OAuth 2.1 required)"
echo "  🔑 OAuth Metadata:    http://localhost:8003 (OAuth endpoints)"
echo
echo "Test tokens:"
echo "  Bearer: test-bearer-token-12345"
echo "  OAuth:  oauth-test-token"
echo
echo "To test discovery, run:"
echo "  curl -X POST http://localhost:8911/scan \\"
echo "    -H 'Content-Type: application/json' \\"
echo "    -d '{\"scanRanges\": [\"127.0.0.1/32\"], \"ports\": [8001,8002,8004]}'"
echo
echo "To stop servers: kill $NO_AUTH_PID $BEARER_PID $OAUTH_PID"
echo "Or run: pkill -f 'python3.*-server.py'"
echo
echo -e "${YELLOW}Press Ctrl+C to stop all servers${NC}"

# Wait for user interrupt
trap 'echo -e "\n${GREEN}Stopping servers...${NC}"; kill $NO_AUTH_PID $BEARER_PID $OAUTH_PID 2>/dev/null; exit 0' INT
wait