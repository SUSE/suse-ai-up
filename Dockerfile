# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o service ./cmd/service

# Final stage
FROM alpine:latest

# Install Python, Node.js, and other dependencies
RUN apk --no-cache add \
    ca-certificates \
    python3 \
    py3-pip \
    nodejs \
    npm \
    curl \
    && ln -sf python3 /usr/bin/python

# Download and install OTEL collector
RUN curl -L https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v0.96.0/otelcol-contrib_0.96.0_linux_amd64.tar.gz \
    | tar -xz -C /usr/local/bin --strip-components=1 otelcol-contrib

# Create non-root user
RUN adduser -D -s /bin/sh -u 1000 mcpuser

# Set working directory
WORKDIR /home/mcpuser/

# Copy the binary from builder stage
COPY --from=builder /app/service .

# Copy swagger files
COPY ./docs/swagger.json ./swagger.json
COPY ./docs/index.html ./swagger.html

# Install Python dependencies for MCP servers
COPY examples/local-mcp/requirements.txt ./requirements.txt
RUN pip3 install --no-cache-dir --upgrade pip && \
    pip3 install --no-cache-dir -r requirements.txt || echo "Warning: Some Python packages failed to install"

# Change ownership to non-root user
RUN chown -R mcpuser:mcpuser /home/mcpuser/

# Switch to non-root user
USER 1000

# Expose ports (main service and OTEL collector)
EXPOSE 8911 4318 4319 8889

# Run the binary
CMD ["./service"]