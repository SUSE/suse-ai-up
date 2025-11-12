# Build stage
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

ARG TARGETARCH

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application for the target architecture
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -a -installsuffix cgo -o service ./cmd/service

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

# Expose main service port
EXPOSE 8911

# Run the binary
CMD ["./service"]