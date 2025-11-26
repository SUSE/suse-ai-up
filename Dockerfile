# Build stage - compile Go binaries for multiple architectures
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

ARG TARGETARCH
ARG BUILDKIT_INLINE_CACHE=1

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary for the target architecture
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s" \
    -o service ./cmd/service

# Final stage - using optimized BCI base image
FROM registry.suse.com/bci/bci-base:16.0

ARG TARGETARCH

# Install only essential runtime dependencies
RUN zypper --non-interactive install --no-recommends \
    ca-certificates \
    python3 \
    python3-pip \
    curl \
    nodejs \
    npm \
    && zypper clean --all \
    && rm -rf /var/cache/zypp/* \
    && rm -rf /tmp/*

# Create Python virtual environment
RUN python3 -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"

# Create non-root user
RUN useradd -r -s /bin/bash -u 1000 mcpuser

# Set working directory
WORKDIR /home/mcpuser/

# Copy the binary from builder stage
COPY --from=builder /app/service .

# Copy swagger files
COPY ./docs/swagger.json ./swagger.json
COPY ./docs/index.html ./swagger.html

# Install Python dependencies in virtual environment
COPY examples/local-mcp/requirements.txt ./requirements.txt
RUN pip install --no-cache-dir --quiet fastmcp==2.11.3 && \
    pip install --no-cache-dir --quiet flask==3.0.0 && \
    pip install --no-cache-dir --quiet flask-cors==4.0.0 || echo "Warning: Some Python packages failed to install"

# Install Node.js dependencies for virtualMCP template
COPY templates/package.json templates/package-lock.json* ./templates/
RUN cd templates && npm ci --only=production && npm cache clean --force && cd ..
RUN npm install -g tsx

# Copy virtualMCP template
COPY templates/virtualmcp-server.ts ./templates/

# Comprehensive cleanup
RUN zypper clean --all \
    && rm -rf /var/cache/zypp/* \
    && rm -rf /tmp/* \
    && rm -rf /var/log/* \
    && find /usr -name "*.pyc" -delete 2>/dev/null || true \
    && find /usr -name "__pycache__" -type d -exec rm -rf {} + 2>/dev/null || true \
    && rm -rf /root/.cache \
    && rm -rf /opt/venv/share/doc \
    && rm -rf /opt/venv/share/man

# Change ownership to non-root user
RUN chown -R mcpuser:mcpuser /home/mcpuser/ /opt/venv/

# Switch to non-root user
USER 1000

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8911/health || exit 1

# Expose main service port
EXPOSE 8911

# Run the binary
CMD ["./service"]