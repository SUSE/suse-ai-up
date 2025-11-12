# Docker Bake configuration for multi-architecture builds
# Usage: docker buildx bake --push

variable "REGISTRY" {
  default = "ghcr.io/alessandro-festa"
}

variable "IMAGE_NAME" {
  default = "suse-ai-up"
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
    "${REGISTRY}/${IMAGE_NAME}:${TAG}",
    "${REGISTRY}/${IMAGE_NAME}:v${TAG}"
  ]
  context = "."
  dockerfile = "Dockerfile"
}

# Target for building only amd64
target "amd64" {
  platforms = ["linux/amd64"]
  tags = ["${REGISTRY}/${IMAGE_NAME}:${TAG}-amd64"]
  context = "."
  dockerfile = "Dockerfile"
}

# Target for building only arm64
target "arm64" {
  platforms = ["linux/arm64"]
  tags = ["${REGISTRY}/${IMAGE_NAME}:${TAG}-arm64"]
  context = "."
  dockerfile = "Dockerfile"
}

# Development build (single platform based on host)
target "dev" {
  tags = ["${REGISTRY}/${IMAGE_NAME}:dev"]
  context = "."
  dockerfile = "Dockerfile"
}