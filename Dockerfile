# Build stage - compile Go binaries for multiple architectures
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

ARG TARGETARCH
ENV CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH}

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary (swagger docs are copied from build context)
RUN go build -ldflags="-w -s" -o suse-ai-up ./cmd

# Final stage - minimal runtime image
FROM alpine:latest

# Install only essential runtime dependencies
RUN apk --no-cache add ca-certificates tzdata kubectl

# Create non-root user
RUN adduser -D -s /bin/sh -u 1000 mcpuser

# Set working directory
WORKDIR /home/mcpuser/

# Copy the binary, docs, and config from builder stage
COPY --from=builder /app/suse-ai-up .
COPY --from=builder /app/config ./config

# Copy swagger docs from build context (generated during make build)
RUN mkdir -p ./docs
COPY docs/swagger.json ./docs/
COPY docs/swagger.yaml ./docs/
COPY docs/docs.go ./docs/
RUN ls -la ./docs/

# Clean up old config files
RUN rm -f config/comprehensive_mcp_servers.yaml*

# Change ownership to non-root user
RUN chown -R mcpuser:mcpuser suse-ai-up docs config

# Switch to non-root user
USER 1000

# Health check - check if the proxy port is responding
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD nc -z localhost 8911 || exit 1

# Expose all service ports (proxy now uses 8911/3911, removed old 8080/38080)
EXPOSE 8911 3911

# Run the binary
CMD ["./suse-ai-up", "uniproxy"]