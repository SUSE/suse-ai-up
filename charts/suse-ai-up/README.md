# SUSE AI Universal Proxy - Helm Chart

![SUSE Logo](https://apps.rancher.io/logos/suse-ai-deployer.png)

This Helm chart deploys the SUSE AI Universal Proxy service with OpenTelemetry observability capabilities and MCP server registry management.

**Certified for SUSE Rancher** | **OpenTelemetry Enabled** | **Multi-Architecture Support**

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- (Optional) Jaeger for distributed tracing
- (Optional) Prometheus for metrics collection

## Installation

### SUSE Rancher Manager UI Installation (Recommended)

1. **Add the SUSE AI Repository** (if not already available):
   - Navigate to **Apps & Marketplace** → **Charts**
   - Search for "SUSE AI Universal Proxy"
   - Click **Install**

2. **Configure via UI Form**:
   - Use the intuitive form to configure your deployment
   - All major settings are available through the categorized interface
   - Advanced options are available in the "Advanced Configuration" section

3. **Deploy**:
   - Review your configuration
   - Click **Install** to deploy to your cluster

### Command Line Installation

### Add Helm Repository (if applicable)

```bash
# Add the repository containing this chart
helm repo add suse-ai-up https://charts.suse.com/ai-up
helm repo update
```

### Install the Chart

```bash
# Install with default values
helm install suse-ai-up ./charts/suse-ai-up

# Install with custom values
helm install suse-ai-up ./charts/suse-ai-up -f my-values.yaml

# Install in a specific namespace
helm install suse-ai-up ./charts/suse-ai-up -n ai-up --create-namespace
```

## Configuration

> 💡 **Tip**: When using SUSE Rancher Manager, most configuration options are available through the intuitive UI form. The table below shows all available parameters for advanced CLI usage.

### Core Service Configuration

| Parameter | Description | Default | Rancher UI Category |
|-----------|-------------|---------|-------------------|
| `image.repository` | Container image repository | `ghcr.io/alessandro-festa/suse-ai-up` | Basic Configuration |
| `image.tag` | Container image tag | `latest` | Basic Configuration |
| `service.port` | Service port | `8911` | Service Configuration |
| `replicaCount` | Number of replicas | `1` | Basic Configuration |

### OpenTelemetry Configuration

| Parameter | Description | Default | Rancher UI Category |
|-----------|-------------|---------|-------------------|
| `otel.enabled` | Enable OpenTelemetry collector sidecar | `false` | OpenTelemetry (OTEL) |
| `otel.serviceName` | Service name for OTEL | `suse-ai-up` | OpenTelemetry (OTEL) |
| `otel.serviceLabels.genai.system.suseai-up` | System identifier label | `true` | OpenTelemetry (OTEL) |
| `otel.exporters.jaeger.enabled` | Enable Jaeger exporter | `true` | OpenTelemetry (OTEL) |
| `otel.exporters.prometheus.enabled` | Enable Prometheus exporter | `true` | OpenTelemetry (OTEL) |
| `otel.app.enabled` | Enable application OTEL instrumentation | `false` | OpenTelemetry (OTEL) |
| `otel.app.endpoint` | OTEL collector endpoint | `http://localhost:4318` | OpenTelemetry (OTEL) |
| `otel.app.protocol` | OTEL protocol (grpc/http) | `grpc` | OpenTelemetry (OTEL) |

### Runtime Support

| Parameter | Description | Default | Rancher UI Category |
|-----------|-------------|---------|-------------------|
| `env.python.enabled` | Enable Python runtime | `true` | Runtime Support |
| `env.nodejs.enabled` | Enable Node.js runtime | `true` | Runtime Support |



### Multi-Architecture Support

This chart supports deployment on both x86_64 (amd64) and ARM64 architectures.

| Parameter | Description | Default | Rancher UI Category |
|-----------|-------------|---------|-------------------|
| `image.architectureTagSuffix` | Architecture-specific tag suffix (e.g., "-amd64", "-arm64") | `""` | Multi-Architecture |
| `architecture` | Target architecture for node affinity ("amd64" or "arm64") | `""` | Multi-Architecture |

### Example Values Override

```yaml
# values.yaml
image:
  tag: "v1.0.0"

replicaCount: 3

otel:
  exporters:
    jaeger:
      endpoint: "jaeger-collector.monitoring:14268/api/traces"

ingress:
  enabled: true
  hosts:
    - host: ai-up.example.com
      paths:
        - path: /
          pathType: Prefix

resources:
  limits:
    cpu: 1000m
    memory: 1Gi
  requests:
    cpu: 500m
    memory: 512Mi
```

### Multi-Architecture Deployment Examples

```yaml
# Deploy on x86_64 nodes only
image:
  architectureTagSuffix: "-amd64"
architecture: "amd64"

# Deploy on ARM64 nodes only
image:
  architectureTagSuffix: "-arm64"
architecture: "arm64"

# Deploy multi-arch image (no suffix, uses manifest)
image:
  architectureTagSuffix: ""  # Empty for multi-arch
architecture: ""  # No affinity constraint
```

## Rancher UI Integration

This Helm chart includes a comprehensive `questions.yaml` file that provides an intuitive form-based interface in SUSE Rancher Manager. The form is organized into logical categories:

- **Basic Configuration**: Image settings, replicas, and core parameters
- **Service Configuration**: Networking and service type options
- **OpenTelemetry (OTEL)**: Collector sidecar and application instrumentation
- **Runtime Support**: Python and Node.js runtime toggles
- **MCP Server Spawning**: Resource limits, retry logic, and spawning configuration
- **Authentication & Security**: Auth modes and service accounts
- **Networking**: Ingress and external access configuration
- **Resources**: CPU and memory allocation
- **Health Checks**: Readiness and liveness probe configuration
- **Multi-Architecture**: Architecture-specific deployment options
- **Advanced Configuration**: Registry, smart agents, and custom settings

The form includes validation, helpful descriptions, and conditional fields that appear based on your selections.

## Features

### 🏢 SUSE Enterprise Ready

- **Certified for SUSE Rancher**: Official SUSE partner certification
- **Enterprise Support**: Backed by SUSE enterprise support
- **Security Hardened**: SUSE security standards and best practices
- **Multi-Architecture**: Support for x86_64 and ARM64 platforms

### 📊 OpenTelemetry Integration

- **Distributed Tracing**: Automatic trace collection with Jaeger export
- **Metrics Collection**: Application and system metrics with Prometheus export
- **Structured Logging**: OTEL-compatible log collection
- **Resource Detection**: Automatic Kubernetes pod metadata detection
- **Application Instrumentation**: Optional in-process OTEL instrumentation
- **Collector Sidecar**: Optional OTEL collector for advanced processing

### 🐍 Runtime Support

- **Python MCP Servers**: Full Python 3.11+ support with pip and common MCP libraries
- **Node.js MCP Servers**: Node.js 18+ with npm for JavaScript/TypeScript MCP servers
- **Container Security**: Non-root execution with minimal attack surface



### ☸️ Kubernetes Native

- **Health Checks**: Readiness and liveness probes
- **Resource Management**: Configurable CPU/memory limits and requests
- **Service Discovery**: Automatic service registration
- **Security Context**: Pod security contexts and RBAC

## Usage

### Accessing the Service

```bash
# Port forward to access locally
kubectl port-forward svc/suse-ai-up 8911:8911

# Access the API
curl http://localhost:8911/health
curl http://localhost:8911/api/v1/adapters
```

### Monitoring

```bash
# Access OTEL collector metrics
kubectl port-forward svc/suse-ai-up-otel 8889:8889

# Access Prometheus metrics
curl http://localhost:8889/metrics
```

### Creating MCP Adapters

```bash
# Create a Python MCP server adapter
curl -X POST http://localhost:8911/api/v1/adapters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "python-mcp-server",
    "protocol": "MCP",
    "connectionType": "LocalStdio",
    "command": "python3",
    "args": ["server.py"],
    "environmentVariables": {
      "PYTHONPATH": "/app"
    }
  }'
```



## Troubleshooting

### Check Pod Status

```bash
kubectl get pods -l app.kubernetes.io/name=suse-ai-up
kubectl describe pod <pod-name>
kubectl logs <pod-name> --container=suse-ai-up
kubectl logs <pod-name> --container=otel-collector
```

### Common Issues

1. **OTEL Collector Not Starting**: Check ConfigMap syntax and resource limits
2. **MCP Server Spawn Failures**: Verify Python/Node.js installations and permissions
3. **Network Issues**: Check service accounts and network policies
4. **Architecture Mismatch**: Ensure image architecture matches node architecture
5. **Multi-Arch Image Issues**: Verify Docker buildx setup and QEMU installation

### Debug Commands

```bash
# Check OTEL collector configuration
kubectl get configmap suse-ai-up-otel-config -o yaml

# Test OTEL collector connectivity
kubectl exec -it <pod-name> -- curl http://localhost:4318/v1/traces

# Check service endpoints
kubectl get endpoints suse-ai-up
```

## Upgrading

```bash
# Upgrade with new values
helm upgrade suse-ai-up ./charts/suse-ai-up -f new-values.yaml

# Rollback if needed
helm rollback suse-ai-up
```

## Uninstalling

```bash
# Uninstall the chart
helm uninstall suse-ai-up

# Clean up PVCs if any
kubectl delete pvc -l app.kubernetes.io/name=suse-ai-up
```

## Development

### Local Testing

```bash
# Lint the chart
helm lint ./charts/suse-ai-up

# Template the chart
helm template test-release ./charts/suse-ai-up

# Install with debug
helm install test-release ./charts/suse-ai-up --debug --dry-run
```

### Building Custom Images

#### Single Architecture Build

```bash
# Build the application
go build -o service ./cmd/service

# Build the Docker image
docker build -t ghcr.io/alessandro-festa/suse-ai-up:latest .
```

#### Multi-Architecture Build

```bash
# Build Go binaries for both architectures
./scripts/build-multiarch.sh

# Build and push multi-architecture Docker images
./scripts/build-multiarch-docker.sh --push

# Or build specific architecture
./scripts/build-multiarch-docker.sh -t amd64 --push
./scripts/build-multiarch-docker.sh -t arm64 --push
```

#### Manual Docker Buildx

```bash
# Set up buildx builder
docker buildx create --name multiarch-builder --use

# Build and push multi-arch image
docker buildx build --platform linux/amd64,linux/arm64 \
  --tag ghcr.io/alessandro-festa/suse-ai-up:latest \
  --push .
```

## Security Considerations

- Run as non-root user (UID 1000)
- Minimal base image (Alpine Linux)
- Network policies recommended
- RBAC for Kubernetes API access
- TLS encryption for production deployments

## Contributing

Please see the main project [CONTRIBUTING.md](../CONTRIBUTING.md) for development guidelines.