# Docker Bake configuration for multi-architecture builds
# Usage: docker buildx bake --push

variable "REGISTRY" {
  default = "ghcr.io"
}

variable "REPO_NAME" {
  default = "alessandro-festa/suse-ai-up"
}

variable "TAG" {
  default = "latest"
}

# Define the target platforms
variable "PLATFORMS" {
  default = ["linux/amd64", "linux/arm64"]
}

# Main target for multi-platform build
target "multiarch" {
  platforms = "${PLATFORMS}"
  tags = [
    "${REGISTRY}/${REPO_NAME}:${TAG}",
    "${REGISTRY}/${REPO_NAME}:v${TAG}"
  ]
  context = "."
  dockerfile = "Dockerfile"
  args = {
    BUILDKIT_INLINE_CACHE = "1"
  }
}

# Target for building only amd64
target "amd64" {
  platforms = ["linux/amd64"]
  tags = ["${REGISTRY}/${REPO_NAME}:${TAG}-amd64"]
  context = "."
  dockerfile = "Dockerfile"
}

# Target for building only arm64
target "arm64" {
  platforms = ["linux/arm64"]
  tags = ["${REGISTRY}/${REPO_NAME}:${TAG}-arm64"]
  context = "."
  dockerfile = "Dockerfile"
}

# Development build (single platform based on host)
target "dev" {
  tags = ["${REGISTRY}/${REPO_NAME}:dev"]
  context = "."
  dockerfile = "Dockerfile"
}

# VirtualMCP server image
target "virtualmcp" {
  platforms = ["linux/amd64", "linux/arm64"]
  tags = [
    "${REGISTRY}/${REPO_NAME}/virtualmcp:latest",
    "${REGISTRY}/${REPO_NAME}/virtualmcp:${TAG}"
  ]
  context = "."
  dockerfile = "Dockerfile.virtualmcp"
  args = {
    BUILDKIT_INLINE_CACHE = "1"
  }
}

# VirtualMCP development build
target "virtualmcp-dev" {
  tags = ["${REGISTRY}/${REPO_NAME}/virtualmcp:dev"]
  context = "."
  dockerfile = "Dockerfile.virtualmcp"
}

# Release target with additional metadata
target "release" {
  inherits = ["multiarch"]
  platforms = "${PLATFORMS}"
  tags = [
    "${REGISTRY}/${REPO_NAME}:${TAG}",
    "${REGISTRY}/${REPO_NAME}:latest",
    "${REGISTRY}/${REPO_NAME}:${TAG}-multiarch"
  ]
  labels = {
    "org.opencontainers.image.title" = "SUSE AI Universal Proxy"
    "org.opencontainers.image.description" = "MCP server proxy and registry"
    "org.opencontainers.image.vendor" = "SUSE"
    "org.opencontainers.image.source" = "https://github.com/${REPO_NAME}"
  }
}