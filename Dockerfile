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

RUN apk --no-cache add ca-certificates

# Create non-root user
RUN adduser -D -s /bin/sh -u 1000 mcpuser

# Set working directory
WORKDIR /home/mcpuser/

# Copy the binary from builder stage
COPY --from=builder /app/service .

# Copy swagger files
COPY ./docs/swagger.json ./swagger.json
COPY ./swagger.html ./swagger.html

# Change ownership to non-root user
RUN chown mcpuser:mcpuser service swagger.json swagger.html

# Switch to non-root user
USER 1000

# Expose port
EXPOSE 8001

# Run the binary
CMD ["./service"]