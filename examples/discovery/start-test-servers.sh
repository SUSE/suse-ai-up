#!/bin/bash

# Enhanced MCP Discovery Test Servers Script
# This script starts all MCP discovery test servers with comprehensive management

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
LOG_DIR="logs"
PID_DIR="pids"

# Create directories for logs and PIDs
mkdir -p "$LOG_DIR" "$PID_DIR"

# Function to show usage
show_usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  -h, --help     Show this help message"
    echo "  -s, --stop     Stop all test servers and exit"
    echo "  -r, --restart  Stop existing servers then start new ones"
    echo "  --status       Show server status and exit"
    echo "  --logs         Show server logs"
    echo "  --clean        Clean up log and PID files"
    echo ""
    echo "Default behavior: Start all test servers"
}

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

# Function to check dependencies
check_dependencies() {
    echo "Checking dependencies..."
    
    # Check Python 3
    if ! command -v python3 >/dev/null 2>&1; then
        echo -e "${RED}✗ Python 3 is required but not installed${NC}"
        exit 1
    fi
    
    # Check required Python packages
    local packages=("fastmcp" "flask" "flask-cors")
    for package in "${packages[@]}"; do
        if ! python3 -c "import $package" 2>/dev/null; then
            echo -e "${RED}✗ Python package '$package' is not installed${NC}"
            echo "Install with: pip install $package"
            exit 1
        fi
    done
    
    # Check if server scripts exist
    local scripts=("no-auth-server.py" "bearer-auth-server.py" "oauth-server.py")
    for script in "${scripts[@]}"; do
        if [ ! -f "$script" ]; then
            echo -e "${RED}✗ Server script '$script' not found${NC}"
            exit 1
        fi
    done
    
    echo -e "${GREEN}✓ All dependencies satisfied${NC}"
}

# Function to check if port is available
check_port() {
    local port=$1
    if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null 2>&1; then
        return 1  # Port is in use
    else
        return 0  # Port is free
    fi
}

# Function to stop existing servers
stop_existing_servers() {
    echo -e "${YELLOW}Stopping existing test servers...${NC}"
    
    # Stop servers using PID files
    for pid_file in "$PID_DIR"/*.pid; do
        if [ -f "$pid_file" ]; then
            local pid=$(cat "$pid_file" 2>/dev/null)
            if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
                kill "$pid" 2>/dev/null || true
                echo -e "${GREEN}✓ Stopped server with PID $pid${NC}"
            fi
            rm -f "$pid_file"
        fi
    done
    
    # Kill any remaining test server processes
    pkill -f "no-auth-server.py" 2>/dev/null || true
    pkill -f "bearer-auth-server.py" 2>/dev/null || true
    pkill -f "oauth-server.py" 2>/dev/null || true
    
    # Force kill if ports are still in use
    for port in 8001 8002 8003 8004; do
        if ! check_port $port; then
            local pid=$(lsof -ti:$port 2>/dev/null)
            if [ -n "$pid" ]; then
                kill -9 "$pid" 2>/dev/null || true
                echo -e "${YELLOW}Force killed process on port $port${NC}"
            fi
        fi
    done
    
    sleep 1
    echo -e "${GREEN}✓ Existing servers stopped${NC}"
}

# Function to wait for server to be ready
wait_for_server() {
    local host_ip=$1
    local port=$2
    local name=$3
    local auth_header=$4
    local count=0
    local max_wait=30

    echo -n "Waiting for $name on port $port..."
    while [ $count -lt $max_wait ]; do
        if [ -n "$auth_header" ]; then
            if curl -s --max-time 2 -H "$auth_header" "http://$host_ip:$port/mcp" >/dev/null 2>&1; then
                echo -e "\n${GREEN}✅ $name is ready on port $port${NC}"
                return 0
            fi
        else
            if curl -s --max-time 2 "http://$host_ip:$port/mcp" >/dev/null 2>&1; then
                echo -e "\n${GREEN}✅ $name is ready on port $port${NC}"
                return 0
            fi
        fi
        
        sleep 1
        ((count++))
        echo -n "."
    done
    
    echo -e "\n${RED}❌ $name failed to start within $max_wait seconds${NC}"
    return 1
}

# Function to start a server
start_server() {
    local script=$1
    local name=$2
    local port=$3
    local auth_header=$4
    
    if ! check_port $port; then
        echo -e "${YELLOW}⚠️  Port $port is already in use, skipping $name${NC}"
        return 0
    fi
    
    echo "Starting $name on port $port..."
    
    # Start server with logging
    python3 "$script" > "$LOG_DIR/$name.log" 2>&1 &
    local pid=$!
    echo $pid > "$PID_DIR/$name.pid"
    
    sleep 2
    
    if wait_for_server "$HOST_IP" "$port" "$name" "$auth_header"; then
        return 0
    else
        echo -e "${RED}✗ $name failed to start${NC}"
        echo "Log output:"
        tail -10 "$LOG_DIR/$name.log"
        return 1
    fi
}

# Function to show server status
show_status() {
    echo ""
    echo -e "${BLUE}Server Status${NC}"
    echo -e "${BLUE}============${NC}"
    
    local servers=(
        "8001:Bearer Auth Server"
        "8002:No-Auth Server"
        "8003:OAuth Auth Server"
        "8004:OAuth MCP Server"
    )
    
    for server_info in "${servers[@]}"; do
        local port="${server_info%%:*}"
        local name="${server_info##*:}"
        
        if check_port $port; then
            echo -e "${RED}✗ Port $port: $name - Inactive${NC}"
        else
            local pid=$(lsof -ti:$port 2>/dev/null)
            echo -e "${GREEN}✓ Port $port: $name - Active (PID: $pid)${NC}"
        fi
    done
}

# Function to show logs
show_logs() {
    echo ""
    echo -e "${BLUE}Recent Server Logs${NC}"
    echo -e "${BLUE}==================${NC}"
    
    for log_file in "$LOG_DIR"/*.log; do
        if [ -f "$log_file" ]; then
            local name=$(basename "$log_file" .log)
            echo ""
            echo -e "${YELLOW}$name:${NC}"
            echo "----------------------------------------"
            tail -5 "$log_file"
        fi
    done
}

# Function to clean up
clean_up() {
    echo "Cleaning up log and PID files..."
    rm -rf "$LOG_DIR" "$PID_DIR"
    echo -e "${GREEN}✓ Cleanup completed${NC}"
}

# Parse command line arguments
STOP_ONLY=false
RESTART=false
SHOW_STATUS=false
SHOW_LOGS=false
CLEAN_UP=false

while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_usage
            exit 0
            ;;
        -s|--stop)
            STOP_ONLY=true
            shift
            ;;
        -r|--restart)
            RESTART=true
            shift
            ;;
        --status)
            SHOW_STATUS=true
            shift
            ;;
        --logs)
            SHOW_LOGS=true
            shift
            ;;
        --clean)
            CLEAN_UP=true
            shift
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            show_usage
            exit 1
            ;;
    esac
done

# Handle different modes
if [ "$SHOW_STATUS" = true ]; then
    show_status
    exit 0
fi

if [ "$SHOW_LOGS" = true ]; then
    show_logs
    exit 0
fi

if [ "$CLEAN_UP" = true ]; then
    stop_existing_servers
    clean_up
    exit 0
fi

if [ "$STOP_ONLY" = true ]; then
    stop_existing_servers
    echo -e "${GREEN}All test servers stopped${NC}"
    exit 0
fi

# Main execution
echo -e "${BLUE}Enhanced MCP Discovery Test Servers${NC}"
echo -e "${BLUE}===================================${NC}"

check_dependencies

# Get host IP
HOST_IP=$(get_host_ip)
echo -e "${GREEN}📡 Using host IP: $HOST_IP${NC}"
echo ""

if [ "$RESTART" = true ]; then
    stop_existing_servers
fi

# Start servers
echo "Starting test servers..."

# Start No-Auth Server (port 8002)
if ! start_server "no-auth-server.py" "no-auth-server" "8002"; then
    exit 1
fi

# Start Bearer Auth Server (port 8001)
if ! start_server "bearer-auth-server.py" "bearer-auth-server" "8001" "Authorization: Bearer test-bearer-token-12345"; then
    exit 1
fi

# Start OAuth Server (ports 8003/8004)
if ! start_server "oauth-server.py" "oauth-server" "8004" "Authorization: Bearer oauth-test-token"; then
    exit 1
fi

# Wait a moment for all servers to be fully ready
sleep 2

# Show final status
echo ""
show_status

echo ""
echo -e "${GREEN}🎉 All test servers started successfully!${NC}"
echo ""
echo "Server Details:"
echo "  📡 No-Auth Server:    http://$HOST_IP:8002/mcp (No authentication)"
echo "  🔐 Bearer Auth Server: http://$HOST_IP:8001/mcp (Bearer token: test-bearer-token-12345)"
echo "  🛡️  OAuth Server:     http://$HOST_IP:8004/mcp (OAuth token: oauth-test-token)"
echo "  🔑 OAuth Auth Server: http://$HOST_IP:8003 (OAuth authorization endpoint)"
echo ""
echo "Management Commands:"
echo "  • Check status: $0 --status"
echo "  • View logs:    $0 --logs"
echo "  • Stop servers: $0 --stop"
echo "  • Restart:      $0 --restart"
echo "  • Clean up:     $0 --clean"
echo ""
echo "Test Discovery:"
echo "  curl -X POST http://localhost:8911/scan \\"
echo "    -H 'Content-Type: application/json' \\"
echo "    -d '{\"scanRanges\": [\"$HOST_IP/32\"], \"ports\": [8001,8002,8004]}'"
echo ""
echo "Log files location: $LOG_DIR/"
echo "PID files location: $PID_DIR/"
echo ""
echo -e "${YELLOW}Press Ctrl+C to stop all servers${NC}"

# Wait for user interrupt
trap 'echo -e "\n${GREEN}Stopping servers...${NC}"; stop_existing_servers; exit 0' INT
wait