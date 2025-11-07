#!/bin/bash

# MCP Discovery Test Script
# This script tests the discovery system's authentication detection capabilities

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
PROXY_URL="http://localhost:8911"
TEST_PORTS="8001,8002,8004"
TIMEOUT=30

# Get host IP
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

HOST_IP=$(get_host_ip)
SCAN_RANGE="${HOST_IP}/32"

echo -e "${BLUE}MCP Discovery Test Script${NC}"
echo -e "${BLUE}========================${NC}"
echo "Host IP: $HOST_IP"
echo "Scan Range: $SCAN_RANGE"
echo "Test Ports: $TEST_PORTS"
echo ""

# Function to check if proxy is running
check_proxy() {
    if ! curl -s "$PROXY_URL/adapters" > /dev/null 2>&1; then
        echo -e "${RED}Error: SUSE AI Universal Proxy is not running on $PROXY_URL${NC}"
        echo "Please start the proxy with: go run cmd/service/main.go"
        exit 1
    fi
    echo -e "${GREEN}✓ Proxy is running${NC}"
}

# Function to check if test servers are running
check_test_servers() {
    echo "Checking test servers..."
    local servers_running=true
    
    # Check no-auth server (port 8002)
    if ! curl -s --max-time 5 "http://localhost:8002/mcp" > /dev/null 2>&1; then
        echo -e "${YELLOW}⚠ No-auth server (port 8002) not running${NC}"
        servers_running=false
    else
        echo -e "${GREEN}✓ No-auth server running${NC}"
    fi
    
    # Check bearer-auth server (port 8001)
    if ! curl -s --max-time 5 -H "Authorization: Bearer test-bearer-token-12345" "http://localhost:8001/mcp" > /dev/null 2>&1; then
        echo -e "${YELLOW}⚠ Bearer-auth server (port 8001) not running${NC}"
        servers_running=false
    else
        echo -e "${GREEN}✓ Bearer-auth server running${NC}"
    fi
    
    # Check OAuth server (port 8004)
    if ! curl -s --max-time 5 -H "Authorization: Bearer oauth-test-token" "http://localhost:8004/mcp" > /dev/null 2>&1; then
        echo -e "${YELLOW}⚠ OAuth server (port 8004) not running${NC}"
        servers_running=false
    else
        echo -e "${GREEN}✓ OAuth server running${NC}"
    fi
    
    if [ "$servers_running" = false ]; then
        echo ""
        echo -e "${YELLOW}Starting missing test servers...${NC}"
        start_test_servers
        sleep 3  # Give servers time to start
    fi
}

# Function to start test servers
start_test_servers() {
    echo "Starting test servers in background..."
    
    # Kill any existing test servers
    pkill -f "no-auth-server.py" 2>/dev/null || true
    pkill -f "bearer-auth-server.py" 2>/dev/null || true
    pkill -f "oauth-server.py" 2>/dev/null || true
    
    # Start servers
    python3 no-auth-server.py >/dev/null 2>&1 &
    NO_AUTH_PID=$!
    sleep 1
    python3 bearer-auth-server.py >/dev/null 2>&1 &
    BEARER_PID=$!
    sleep 1
    python3 oauth-server.py >/dev/null 2>&1 &
    OAUTH_PID=$!
    sleep 2
    
    echo -e "${GREEN}✓ Test servers started${NC}"
}

# Function to run discovery scan
run_discovery_scan() {
    echo ""
    echo -e "${BLUE}Running MCP discovery scan...${NC}"
    
    local scan_response=$(curl -s -X POST "$PROXY_URL/scan" \
        -H "Content-Type: application/json" \
        -d "{\"scanRanges\": [\"$SCAN_RANGE\"], \"ports\": [$TEST_PORTS]}" \
        --max-time $TIMEOUT)
    
    echo "Scan Response:"
    echo "$scan_response" | jq '.' 2>/dev/null || echo "$scan_response"
    
    # Extract scan ID
    local scan_id=$(echo "$scan_response" | jq -r '.scanId // empty' 2>/dev/null)
    if [ -n "$scan_id" ] && [ "$scan_id" != "null" ]; then
        echo -e "${GREEN}✓ Scan completed with ID: $scan_id${NC}"
        return 0
    else
        echo -e "${RED}✗ Scan failed${NC}"
        return 1
    fi
}

# Function to list discovered servers
list_discovered_servers() {
    echo ""
    echo -e "${BLUE}Listing discovered servers...${NC}"
    
    local servers_response=$(curl -s "$PROXY_URL/servers")
    echo "Discovered Servers:"
    echo "$servers_response" | jq '.' 2>/dev/null || echo "$servers_response"
    
    # Count servers
    local server_count=$(echo "$servers_response" | jq '. | length' 2>/dev/null || echo "0")
    echo -e "${GREEN}✓ Found $server_count servers${NC}"
}

# Function to verify authentication types
verify_auth_types() {
    echo ""
    echo -e "${BLUE}Verifying authentication types...${NC}"
    
    local servers_response=$(curl -s "$PROXY_URL/servers")
    local auth_types_correct=true
    
    # Check for expected auth types
    local has_no_auth=$(echo "$servers_response" | jq -r '.[] | select(.address | contains("8002")) | .metadata.auth_type // "none"' 2>/dev/null)
    local has_bearer=$(echo "$servers_response" | jq -r '.[] | select(.address | contains("8001")) | .metadata.auth_type // "none"' 2>/dev/null)
    local has_oauth=$(echo "$servers_response" | jq -r '.[] | select(.address | contains("8004")) | .metadata.auth_type // "none"' 2>/dev/null)
    
    echo "Authentication Types Found:"
    echo "Port 8002 (No Auth): $has_no_auth"
    echo "Port 8001 (Bearer): $has_bearer"
    echo "Port 8004 (OAuth): $has_oauth"
    
    # Verify expected values
    if [ "$has_no_auth" = "none" ] || [ "$has_no_auth" = '"none"' ]; then
        echo -e "${GREEN}✓ No-auth server correctly detected${NC}"
    else
        echo -e "${RED}✗ No-auth server detection failed${NC}"
        auth_types_correct=false
    fi
    
    if [ "$has_bearer" = "bearer" ] || [ "$has_bearer" = '"bearer"' ]; then
        echo -e "${GREEN}✓ Bearer-auth server correctly detected${NC}"
    else
        echo -e "${RED}✗ Bearer-auth server detection failed${NC}"
        auth_types_correct=false
    fi
    
    if [ "$has_oauth" = "oauth" ] || [ "$has_oauth" = '"oauth"' ]; then
        echo -e "${GREEN}✓ OAuth server correctly detected${NC}"
    else
        echo -e "${RED}✗ OAuth server detection failed${NC}"
        auth_types_correct=false
    fi
    
    return $([ "$auth_types_correct" = true ] && echo 0 || echo 1)
}

# Function to verify vulnerability scoring
verify_vulnerability_scoring() {
    echo ""
    echo -e "${BLUE}Verifying vulnerability scoring...${NC}"
    
    local servers_response=$(curl -s "$PROXY_URL/servers")
    local scoring_correct=true
    
    # Check vulnerability scores
    local no_auth_score=$(echo "$servers_response" | jq -r '.[] | select(.address | contains("8002")) | .vulnerability_score // "unknown"' 2>/dev/null)
    local bearer_score=$(echo "$servers_response" | jq -r '.[] | select(.address | contains("8001")) | .vulnerability_score // "unknown"' 2>/dev/null)
    local oauth_score=$(echo "$servers_response" | jq -r '.[] | select(.address | contains("8004")) | .vulnerability_score // "unknown"' 2>/dev/null)
    
    echo "Vulnerability Scores:"
    echo "Port 8002 (No Auth): $no_auth_score (expected: high)"
    echo "Port 8001 (Bearer): $bearer_score (expected: medium)"
    echo "Port 8004 (OAuth): $oauth_score (expected: low)"
    
    # Verify expected scores
    if [ "$no_auth_score" = "high" ] || [ "$no_auth_score" = '"high"' ]; then
        echo -e "${GREEN}✓ No-auth server correctly scored as high vulnerability${NC}"
    else
        echo -e "${RED}✗ No-auth server vulnerability scoring failed${NC}"
        scoring_correct=false
    fi
    
    if [ "$bearer_score" = "medium" ] || [ "$bearer_score" = '"medium"' ]; then
        echo -e "${GREEN}✓ Bearer-auth server correctly scored as medium vulnerability${NC}"
    else
        echo -e "${RED}✗ Bearer-auth server vulnerability scoring failed${NC}"
        scoring_correct=false
    fi
    
    if [ "$oauth_score" = "low" ] || [ "$oauth_score" = '"low"' ]; then
        echo -e "${GREEN}✓ OAuth server correctly scored as low vulnerability${NC}"
    else
        echo -e "${RED}✗ OAuth server vulnerability scoring failed${NC}"
        scoring_correct=false
    fi
    
    return $([ "$scoring_correct" = true ] && echo 0 || echo 1)
}

# Function to test manual connectivity
test_manual_connectivity() {
    echo ""
    echo -e "${BLUE}Testing manual server connectivity...${NC}"
    
    # Test no-auth server
    echo "Testing no-auth server (port 8002)..."
    if curl -s --max-time 5 -X POST "http://localhost:8002/mcp" \
        -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | grep -q "result"; then
        echo -e "${GREEN}✓ No-auth server accessible${NC}"
    else
        echo -e "${RED}✗ No-auth server not accessible${NC}"
    fi
    
    # Test bearer-auth server without token
    echo "Testing bearer-auth server without token (should fail)..."
    if curl -s --max-time 5 -X POST "http://localhost:8001/mcp" \
        -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | grep -q "error\|401\|403"; then
        echo -e "${GREEN}✓ Bearer-auth server correctly rejects unauthenticated requests${NC}"
    else
        echo -e "${RED}✗ Bearer-auth server should reject unauthenticated requests${NC}"
    fi
    
    # Test bearer-auth server with token
    echo "Testing bearer-auth server with token..."
    if curl -s --max-time 5 -X POST "http://localhost:8001/mcp" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer test-bearer-token-12345" \
        -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | grep -q "result"; then
        echo -e "${GREEN}✓ Bearer-auth server accessible with valid token${NC}"
    else
        echo -e "${RED}✗ Bearer-auth server not accessible with valid token${NC}"
    fi
    
    # Test OAuth server without token
    echo "Testing OAuth server without token (should fail)..."
    if curl -s --max-time 5 -X POST "http://localhost:8004/mcp" \
        -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | grep -q "error\|401\|403"; then
        echo -e "${GREEN}✓ OAuth server correctly rejects unauthenticated requests${NC}"
    else
        echo -e "${RED}✗ OAuth server should reject unauthenticated requests${NC}"
    fi
    
    # Test OAuth server with token
    echo "Testing OAuth server with token..."
    if curl -s --max-time 5 -X POST "http://localhost:8004/mcp" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer oauth-test-token" \
        -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | grep -q "result"; then
        echo -e "${GREEN}✓ OAuth server accessible with valid token${NC}"
    else
        echo -e "${RED}✗ OAuth server not accessible with valid token${NC}"
    fi
}

# Function to generate test report
generate_report() {
    echo ""
    echo -e "${BLUE}Generating test report...${NC}"
    
    local timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    local report_file="discovery-test-report-$timestamp.json"
    
    local servers_response=$(curl -s "$PROXY_URL/servers")
    
    cat > "$report_file" << EOF
{
  "testTimestamp": "$timestamp",
  "hostIp": "$HOST_IP",
  "scanRange": "$SCAN_RANGE",
  "testPorts": "$TEST_PORTS",
  "discoveryResults": $servers_response,
  "summary": {
    "totalServersFound": $(echo "$servers_response" | jq '. | length' 2>/dev/null || echo "0"),
    "authenticationTypesDetected": {
      "none": $(echo "$servers_response" | jq '[.[] | select(.metadata.auth_type == "none")] | length' 2>/dev/null || echo "0"),
      "bearer": $(echo "$servers_response" | jq '[.[] | select(.metadata.auth_type == "bearer")] | length' 2>/dev/null || echo "0"),
      "oauth": $(echo "$servers_response" | jq '[.[] | select(.metadata.auth_type == "oauth")] | length' 2>/dev/null || echo "0")
    },
    "vulnerabilityScores": {
      "high": $(echo "$servers_response" | jq '[.[] | select(.vulnerability_score == "high")] | length' 2>/dev/null || echo "0"),
      "medium": $(echo "$servers_response" | jq '[.[] | select(.vulnerability_score == "medium")] | length' 2>/dev/null || echo "0"),
      "low": $(echo "$servers_response" | jq '[.[] | select(.vulnerability_score == "low")] | length' 2>/dev/null || echo "0")
    }
  }
}
EOF
    
    echo -e "${GREEN}✓ Test report saved to: $report_file${NC}"
    echo "Report contents:"
    cat "$report_file" | jq '.' 2>/dev/null || cat "$report_file"
}

# Function to cleanup
cleanup() {
    echo ""
    echo -e "${BLUE}Cleaning up...${NC}"
    
    # Stop test servers
    if [ -n "$NO_AUTH_PID" ]; then
        kill $NO_AUTH_PID 2>/dev/null || true
    fi
    if [ -n "$BEARER_PID" ]; then
        kill $BEARER_PID 2>/dev/null || true
    fi
    if [ -n "$OAUTH_PID" ]; then
        kill $OAUTH_PID 2>/dev/null || true
    fi
    
    # Also kill any remaining processes
    pkill -f "no-auth-server.py" 2>/dev/null || true
    pkill -f "bearer-auth-server.py" 2>/dev/null || true
    pkill -f "oauth-server.py" 2>/dev/null || true
    
    echo -e "${GREEN}✓ Test servers stopped${NC}"
}

# Main execution
main() {
    echo -e "${BLUE}Starting MCP Discovery Test Suite${NC}"
    echo "=================================="
    
    # Check prerequisites
    check_proxy
    check_test_servers
    
    # Run tests
    local tests_passed=true
    
    if ! run_discovery_scan; then
        tests_passed=false
    fi
    
    list_discovered_servers
    
    if ! verify_auth_types; then
        tests_passed=false
    fi
    
    if ! verify_vulnerability_scoring; then
        tests_passed=false
    fi
    
    test_manual_connectivity
    generate_report
    
    # Final result
    echo ""
    echo "=================================="
    if [ "$tests_passed" = true ]; then
        echo -e "${GREEN}✓ All tests passed successfully!${NC}"
    else
        echo -e "${RED}✗ Some tests failed. Check the output above for details.${NC}"
    fi
    
    cleanup
}

# Handle script interruption
trap cleanup EXIT

# Run main function
main "$@"