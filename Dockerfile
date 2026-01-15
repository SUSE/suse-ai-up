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

# Build the unified binary
RUN go build -ldflags="-w -s" -o suse-ai-up ./cmd/uniproxy

# Final stage - minimal runtime image
FROM registry.suse.com/bci/bci-base:16.0

# Install only essential runtime dependencies
RUN zypper --non-interactive install ca-certificates timezone

# Create non-root user
RUN useradd -r -s /bin/bash -u 1000 mcpuser

WORKDIR /home/mcpuser/

# Copy binary, config, and docs
COPY --from=builder /app/suse-ai-up .
COPY --from=builder /app/config ./config
COPY --from=builder /app/docs ./docs

# Clean up and set permissions
RUN rm -f config/comprehensive_mcp_servers.yaml*

RUN chown -R mcpuser:mcpuser suse-ai-up config

# Switch to non-root user
USER 1000

# Health check - check if the proxy port is responding
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD nc -z localhost 8911 || exit 1

# Expose unified service ports
EXPOSE 8911 3911

# Run the unified binary
CMD ["./suse-ai-up"]