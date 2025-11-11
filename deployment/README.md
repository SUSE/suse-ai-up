# SUSE AI Universal Proxy - Helm Chart

This Helm chart deploys the SUSE AI Universal Proxy service with OpenTelemetry observability capabilities and support for spawning Python and Node.js MCP servers.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- (Optional) Jaeger for distributed tracing
- (Optional) Prometheus for metrics collection

## Installation

### Add Helm Repository (if applicable)

```bash
# Add the repository containing this chart
helm repo add suse-ai-up https://charts.suse.com/ai-up
helm repo update
```

### Install the Chart

```bash
# Install with default values
helm install suse-ai-up ./deployment

# Install with custom values
helm install suse-ai-up ./deployment -f my-values.yaml

# Install in a specific namespace
helm install suse-ai-up ./deployment -n ai-up --create-namespace
```

## Configuration

### Core Service Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | Container image repository | `ghcr.io/alessandro-festa/suse-ai-up` |
| `image.tag` | Container image tag | `latest` |
| `service.port` | Service port | `8911` |
| `replicaCount` | Number of replicas | `1` |

### OpenTelemetry Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `otel.enabled` | Enable OpenTelemetry | `true` |
| `otel.serviceName` | Service name for OTEL | `suse-ai-up` |
| `otel.exporters.jaeger.enabled` | Enable Jaeger exporter | `true` |
| `otel.exporters.prometheus.enabled` | Enable Prometheus exporter | `true` |

### Runtime Support

| Parameter | Description | Default |
|-----------|-------------|---------|
| `env.python.enabled` | Enable Python runtime | `true` |
| `env.nodejs.enabled` | Enable Node.js runtime | `true` |

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

## Features

### OpenTelemetry Integration

- **Distributed Tracing**: Automatic trace collection with Jaeger export
- **Metrics Collection**: Application and system metrics with Prometheus export
- **Structured Logging**: OTEL-compatible log collection
- **Resource Detection**: Automatic Kubernetes pod metadata detection

### Runtime Support

- **Python MCP Servers**: Full Python 3.11+ support with pip and common MCP libraries
- **Node.js MCP Servers**: Node.js 18+ with npm for JavaScript/TypeScript MCP servers
- **Container Security**: Non-root execution with minimal attack surface

### Kubernetes Native

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
helm upgrade suse-ai-up ./deployment -f new-values.yaml

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
helm lint ./deployment

# Template the chart
helm template test-release ./deployment

# Install with debug
helm install test-release ./deployment --debug --dry-run
```

### Building Custom Images

```bash
# Build the application
go build -o service ./cmd/service

# Build the Docker image
docker build -t ghcr.io/alessandro-festa/suse-ai-up:latest .
```

## Security Considerations

- Run as non-root user (UID 1000)
- Minimal base image (Alpine Linux)
- Network policies recommended
- RBAC for Kubernetes API access
- TLS encryption for production deployments

## Contributing

Please see the main project [CONTRIBUTING.md](../CONTRIBUTING.md) for development guidelines.