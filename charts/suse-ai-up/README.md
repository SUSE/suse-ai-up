# SUSE AI Universal Proxy

![Version: 1.0.0](https://img.shields.io/badge/Version-1.0.0-informational?style=flat-square)
![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square)
![AppVersion: 1.0.0](https://img.shields.io/badge/AppVersion-1.0.0-informational?style=flat-square)

A comprehensive, modular MCP (Model Context Protocol) proxy system that enables secure, scalable, and extensible AI model integrations.

## TL;DR

```bash
# Install with LoadBalancer for external access
helm install suse-ai-up ./charts/suse-ai-up --set service.type=LoadBalancer

# Create MCP adapter with sidecar deployment
curl -X POST "http://<SERVICE-IP>:8911/api/v1/adapters" \
  -H "Content-Type: application/json" \
  -d '{"mcpServerId": "uyuni", "name": "uyuni-adapter", "environmentVariables": {"UYUNI_SERVER": "https://uyuni.example.com", "UYUNI_USER": "admin", "UYUNI_PASS": "password"}}'
```

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+

## Installing the Chart

To install the chart with the release name `suse-ai-up`:

```bash
helm install suse-ai-up ./charts/suse-ai-up
```

Alternatively, if you have the chart packaged:

```bash
helm install suse-ai-up suse-ai-up-0.0.1.tgz
```

## Uninstalling the Chart

To uninstall/delete the `suse-ai-up` deployment:

```bash
helm uninstall suse-ai-up
```

## Configuration

The following table lists the configurable parameters of the SUSE AI Universal Proxy chart and their default values.

### Global Parameters

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| global.imageRegistry | string | `""` | Global Docker image registry |
| global.imagePullSecrets | list | `[]` | Global Docker registry secret names |
| global.storageClass | string | `""` | Global storage class for persistent volumes |

### Common Parameters

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| image.registry | string | `"ghcr.io"` | SUSE AI Universal Proxy image registry |
| image.repository | string | `"alessandro-festa/suse-ai-up"` | SUSE AI Universal Proxy image repository |
| image.tag | string | `""` | SUSE AI Universal Proxy image tag (defaults to appVersion) |
| image.pullPolicy | string | `"IfNotPresent"` | SUSE AI Universal Proxy image pull policy |
| image.architecture | list | `["amd64", "arm64"]` | Supported architectures |

### Service Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| service.type | string | `"ClusterIP"` | Service type (ClusterIP, LoadBalancer, NodePort) |
| service.annotations | object | `{}` | Additional service annotations |
| services.proxy.enabled | bool | `true` | Enable MCP proxy service |
| services.proxy.port | int | `8080` | HTTP port for proxy service |
| services.proxy.tlsPort | int | `38080` | HTTPS port for proxy service |
| services.registry.enabled | bool | `true` | Enable MCP registry service |
| services.registry.port | int | `8913` | HTTP port for registry service |
| services.registry.tlsPort | int | `38913` | HTTPS port for registry service |
| services.discovery.enabled | bool | `true` | Enable network discovery service |
| services.discovery.port | int | `8912` | HTTP port for discovery service |
| services.discovery.tlsPort | int | `38912` | HTTPS port for discovery service |
| services.plugins.enabled | bool | `true` | Enable plugin management service |
| services.plugins.port | int | `8914` | HTTP port for plugins service |
| services.plugins.tlsPort | int | `38914` | HTTPS port for plugins service |

### Registry Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| registry.enabled | bool | `true` | Enable MCP server registry |
| registry.url | string | `""` | External URL for registry YAML file (leave empty for ConfigMap) |
| registry.timeout | string | `"30s"` | Timeout for fetching external registry |

### TLS Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| tls.enabled | bool | `true` | Enable TLS encryption |
| tls.autoGenerate | bool | `true` | Auto-generate self-signed certificates |
| tls.certFile | string | `""` | Path to custom TLS certificate |
| tls.keyFile | string | `""` | Path to custom TLS private key |
| tls.secretName | string | `""` | Name of existing TLS secret |

### Authentication Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| auth.method | string | `"oauth"` | Authentication method (oauth, bearer, apikey, basic, none) |
| auth.oauth.enabled | bool | `false` | Enable OAuth 2.0 authentication |
| auth.oauth.clientId | string | `""` | OAuth client ID |
| auth.oauth.clientSecret | string | `""` | OAuth client secret |
| auth.apikey.enabled | bool | `false` | Enable API key authentication |
| auth.apikey.header | string | `"X-API-Key"` | API key header name |
| auth.bearer.enabled | bool | `false` | Enable Bearer token authentication |

### Monitoring Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| monitoring.enabled | bool | `false` | Enable monitoring integration |
| monitoring.prometheus.enabled | bool | `false` | Connect to existing Prometheus |
| monitoring.prometheus.existingService | string | `""` | Name of existing Prometheus service |
| monitoring.grafana.enabled | bool | `false` | Create dashboard for existing Grafana |
| monitoring.grafana.existingService | string | `""` | Name of existing Grafana service |

### Resource Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| services.proxy.resources.requests.cpu | string | `"100m"` | Proxy CPU request |
| services.proxy.resources.requests.memory | string | `"128Mi"` | Proxy memory request |
| services.proxy.resources.limits.cpu | string | `"500m"` | Proxy CPU limit |
| services.proxy.resources.limits.memory | string | `"512Mi"` | Proxy memory limit |

*(Similar resource configurations exist for registry, discovery, and plugins services)*

### Ingress Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| ingress.enabled | bool | `false` | Enable ingress |
| ingress.className | string | `""` | Ingress class name |
| ingress.hosts | list | `[]` | List of ingress hosts |

### Service Account Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| serviceAccount.create | bool | `true` | Create service account |
| serviceAccount.annotations | object | `{}` | Service account annotations |
| serviceAccount.name | string | `""` | Service account name |

## Examples

### Basic Installation

```bash
helm install suse-ai-up suse/suse-ai-up \
  --set services.proxy.enabled=true \
  --set services.registry.enabled=true \
  --set tls.enabled=true
```

### Custom TLS Certificates

```bash
helm install suse-ai-up suse/suse-ai-up \
  --set tls.certFile=/path/to/cert.pem \
  --set tls.keyFile=/path/to/key.pem \
   --set tls.secretName=my-tls-secret
```

### Different Service Types

#### LoadBalancer (for cloud environments)
```bash
helm install suse-ai-up ./charts/suse-ai-up \
  --set service.type=LoadBalancer
```

#### NodePort (for on-premises)
```bash
helm install suse-ai-up ./charts/suse-ai-up \
  --set service.type=NodePort
```

#### ClusterIP (default, for internal access)
```bash
helm install suse-ai-up ./charts/suse-ai-up \
  --set service.type=ClusterIP
```

### External Registry Configuration

Use an external registry URL for dynamic registry management:

```bash
# Install with GitHub-hosted registry
helm install suse-ai-up ./charts/suse-ai-up \
  --set registry.url="https://raw.githubusercontent.com/your-org/mcp-registry/main/mcp_registry.yaml"

# Reload registry at runtime (after updating the external file)
curl -X POST "http://<SERVICE-IP>:8911/api/v1/registry/reload"
```

**Registry URL Requirements:**
- Must serve valid YAML content
- File structure should match the MCP registry format
- Supports any URL (GitHub, GitLab, HTTP server, etc.)
- Falls back to ConfigMap if URL is unavailable

### With Monitoring

First, install Prometheus and Grafana separately:

```bash
# Install Prometheus Operator (includes Prometheus)
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm install prometheus prometheus-community/kube-prometheus-stack

# Install Grafana (optional)
helm repo add grafana https://grafana.github.io/helm-charts
helm install grafana grafana/grafana
```

Then install SUSE AI Universal Proxy with monitoring:

```bash
helm install suse-ai-up suse/suse-ai-up \
  --set monitoring.enabled=true \
  --set monitoring.prometheus.enabled=true \
  --set monitoring.grafana.enabled=true
```

### High Availability Setup

```yaml
replicaCount: 3
services:
  proxy:
    resources:
      requests:
        cpu: 200m
        memory: 256Mi
      limits:
        cpu: 1000m
        memory: 1Gi
  registry:
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
      limits:
        cpu: 500m
        memory: 512Mi
```

## Service Ports

After installation, the services will be available on the following ports:

| Service | HTTP Port | HTTPS Port | Description |
|---------|-----------|------------|-------------|
| Proxy | 8080 | 38080 | MCP protocol proxy |
| Registry | 8913 | 38913 | Server catalog management |
| Discovery | 8912 | 38912 | Network scanning service |
| Plugins | 8914 | 38914 | Plugin management |
| Health/Docs | 8911 | 3911 | Health checks and API docs |

## Accessing Services

### API Documentation
```bash
# Open Swagger UI
open http://your-cluster-ip:8911/docs

# Reload registry from configured URL
curl -X POST "http://your-cluster-ip:8911/api/v1/registry/reload"
```

### Health Checks
```bash
# Check all services
curl http://your-cluster-ip:8911/health

# Individual service checks
curl http://your-cluster-ip:8080/health  # Proxy
curl http://your-cluster-ip:8913/health  # Registry
curl http://your-cluster-ip:8912/health  # Discovery
curl http://your-cluster-ip:8914/health  # Plugins
```

## Troubleshooting

### Common Issues

**Pods not starting:**
```bash
kubectl get pods -l app.kubernetes.io/name=suse-ai-up
kubectl describe pod <pod-name>
kubectl logs <pod-name>
```

**Service unavailable:**
```bash
kubectl get services -l app.kubernetes.io/name=suse-ai-up
kubectl describe service <service-name>
```

**TLS certificate issues:**
```bash
# Check certificate validity
kubectl get secrets -l app.kubernetes.io/name=suse-ai-up
kubectl describe secret <tls-secret-name>
```

### Logs and Debugging

```bash
# View all pod logs
kubectl logs -l app.kubernetes.io/name=suse-ai-up --all-containers

# View specific service logs
kubectl logs -l app.kubernetes.io/name=suse-ai-up -c proxy

# Debug with temporary pod
kubectl run debug --image=busybox --rm -it -- sh
```

## Upgrading

To upgrade the chart:

```bash
helm upgrade suse-ai-up suse/suse-ai-up
```

To upgrade with new values:

```bash
helm upgrade suse-ai-up suse/suse-ai-up -f new-values.yaml
```

## Backup and Restore

*(Currently not implemented - handled externally)*

## Security Considerations

- TLS encryption enabled by default
- Self-signed certificates for development
- Configurable authentication methods
- Pod security contexts applied
- Network policies can be added

## Support

- **Documentation**: [SUSE AI Universal Proxy Docs](https://github.com/suse/suse-ai-up/tree/main/docs)
- **Issues**: [GitHub Issues](https://github.com/suse/suse-ai-up/issues)
- **Discussions**: [GitHub Discussions](https://github.com/suse/suse-ai-up/discussions)

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](https://github.com/suse/suse-ai-up/blob/main/LICENSE) for details.