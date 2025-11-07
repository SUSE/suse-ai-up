# Multi-stage Dockerfile for MCP Discovery Test Servers
FROM python:3.12-slim

# Set working directory
WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements and install Python packages
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy server scripts
COPY *.py ./

# Create non-root user for security
RUN useradd -m -u 1000 mcpuser && chown -R mcpuser:mcpuser /app
USER mcpuser

# Expose ports
EXPOSE 8001 8002 8003 8004

# Health check script
COPY <<EOF /app/healthcheck.sh
#!/bin/sh
# Health check script for different server types
if [ "\$PORT" = "8001" ]; then
    # Bearer auth server
    curl -f -H "Authorization: Bearer test-bearer-token-12345" http://localhost:8001/mcp > /dev/null 2>&1
elif [ "\$PORT" = "8002" ]; then
    # No auth server
    curl -f http://localhost:8002/mcp > /dev/null 2>&1
elif [ "\$PORT" = "8003" ]; then
    # OAuth auth server
    curl -f http://localhost:8003/.well-known/oauth-authorization-server > /dev/null 2>&1
elif [ "\$PORT" = "8004" ]; then
    # OAuth MCP server
    curl -f -H "Authorization: Bearer oauth-test-token" http://localhost:8004/mcp > /dev/null 2>&1
fi
EOF

RUN chmod +x /app/healthcheck.sh

# Default command (can be overridden)
CMD ["python3", "no-auth-server.py"]

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD /app/healthcheck.sh