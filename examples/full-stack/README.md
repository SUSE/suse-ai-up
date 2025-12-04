# Full Stack Example

This example demonstrates a complete SUSE AI Universal Proxy deployment with all services running in a production-like configuration. It includes the proxy, registry, discovery, and plugins services with proper service mesh architecture.

## Overview

The full stack setup includes:
- **Proxy Service**: Main entry point with load balancing
- **Registry Service**: MCP server catalog and discovery
- **Discovery Service**: Automated network scanning
- **Plugins Service**: Plugin lifecycle management
- **Service Mesh**: Proper inter-service communication
- **Monitoring**: Health checks and metrics
- **Security**: TLS encryption and authentication

## Architecture

```
┌─────────────┐
│   CLIENT    │
│  (Browser,  │
│   CLI, IDE) │
└──────┬──────┘
       │
       ▼ HTTPS (38080)
┌─────────────┐
│   PROXY     │◄─────────────────┐
│  (Gateway)  │                  │
│ Port: 8080  │                  │
│ HTTPS:38080 │                  │
└──────┬──────┘                  │
       │                         │
       ▼                         │
┌─────────────┐    ┌─────────────┐│
│  REGISTRY   │◄──►│ DISCOVERY   ││
│ (Catalog)   │    │ (Scanner)   ││
│ Port: 8913  │    │ Port: 8912  ││
│ HTTPS:38913 │    │ HTTPS:38912 ││
└──────┬──────┘    └─────────────┘│
       │                         │
       ▼                         │
┌─────────────┐                  │
│  PLUGINS    │◄─────────────────┘
│ (Manager)   │
│ Port: 8914  │
│ HTTPS:38914 │
└──────┬──────┘
       │
       ▼
┌─────────────┐
│ MCP SERVERS │
│  (External) │
└─────────────┘
```

## Quick Start

### Using Docker Compose

```bash
# Start the full stack
docker-compose up -d

# Check service health
curl http://localhost:8911/health

# Access Swagger documentation
open http://localhost:8911/docs
```

### Using Helm (Kubernetes)

```bash
# Add the Helm repository
helm repo add suse-ai-up https://charts.suse.ai

# Install with default configuration
helm install suse-ai-up suse-ai-up/suse-ai-up

# Check deployment status
kubectl get pods
kubectl get services
```

## Configuration

### Environment Variables

```bash
# Global Configuration
LOG_LEVEL=info
AUTO_TLS=true

# Proxy Service
PROXY_PORT=8080
TLS_PORT=38080
AUTH_MODE=oauth

# Registry Service
REGISTRY_PORT=8913
TLS_REGISTRY_PORT=38913
ENABLE_OFFICIAL=true
ENABLE_DOCKER=true

# Discovery Service
DISCOVERY_PORT=8912
TLS_DISCOVERY_PORT=38912
SCAN_INTERVAL=300

# Plugins Service
PLUGINS_PORT=8914
TLS_PLUGINS_PORT=38914
HEALTH_INTERVAL=30
```

### Docker Compose Configuration

```yaml
version: '3.8'

services:
  proxy:
    image: suse/suse-ai-up:latest
    ports:
      - "8080:8080"
      - "38080:38080"
    environment:
      - AUTH_MODE=oauth
      - AUTO_TLS=true
      - REGISTRY_URL=http://registry:8913
    depends_on:
      - registry
    command: ["./suse-ai-up", "proxy"]

  registry:
    image: suse/suse-ai-up:latest
    ports:
      - "8913:8913"
      - "38913:38913"
    environment:
      - AUTO_TLS=true
      - ENABLE_OFFICIAL=true
      - ENABLE_DOCKER=true
    command: ["./suse-ai-up", "registry"]

  discovery:
    image: suse/suse-ai-up:latest
    ports:
      - "8912:8912"
      - "38912:38912"
    environment:
      - AUTO_TLS=true
      - REGISTRY_URL=http://registry:8913
    depends_on:
      - registry
    command: ["./suse-ai-up", "discovery"]

  plugins:
    image: suse/suse-ai-up:latest
    ports:
      - "8914:8914"
      - "38914:38914"
    environment:
      - AUTO_TLS=true
      - REGISTRY_URL=http://registry:8913
    depends_on:
      - registry
    command: ["./suse-ai-up", "plugins"]
```

## Service Startup Order

The services must start in the correct order to ensure proper initialization:

1. **Registry** (8913) - Must be first for service discovery
2. **Discovery** (8912) - Depends on registry for server registration
3. **Plugins** (8914) - Depends on registry for plugin catalog
4. **Proxy** (8080) - Depends on all services for routing

## Authentication Setup

### OAuth 2.0 Configuration

```bash
# Azure AD Example
export AUTH_MODE=oauth
export OAUTH_PROVIDER=azure
export OAUTH_CLIENT_ID=your-client-id
export OAUTH_CLIENT_SECRET=your-client-secret
export OAUTH_TENANT_ID=your-tenant-id

# Google OAuth Example
export AUTH_MODE=oauth
export OAUTH_PROVIDER=google
export OAUTH_CLIENT_ID=your-client-id
export OAUTH_CLIENT_SECRET=your-client-secret

# Okta Example
export AUTH_MODE=oauth
export OAUTH_PROVIDER=okta
export OAUTH_CLIENT_ID=your-client-id
export OAUTH_CLIENT_SECRET=your-client-secret
export OAUTH_ISSUER=https://your-org.okta.com/oauth2/default
```

### Bearer Token Configuration

```bash
export AUTH_MODE=bearer
export AUTO_GENERATE_BEARER=true
export TOKEN_EXPIRES_HOURS=24
```

## Registry Configuration

### Official Registry Sync

```yaml
registry:
  sync:
    official:
      enabled: true
      interval: 24h
      url: "https://registry.mcp-project.org/api/v1/servers"
    docker:
      enabled: true
      interval: 6h
      registries:
        - "docker.io"
        - "registry.example.com"
```

### Custom Server Registration

```bash
# Register a custom MCP server
curl -X POST http://localhost:8913/api/v1/registry/upload \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My Custom Server",
    "description": "Custom MCP server implementation",
    "packages": [{
      "name": "custom-tools",
      "version": "1.0.0",
      "transport": {
        "type": "http",
        "url": "http://my-server:8080"
      }
    }]
  }'
```

## Discovery Configuration

### Network Scanning Setup

```yaml
discovery:
  scan:
    enabled: true
    interval: 300  # 5 minutes
    networks:
      - "192.168.1.0/24"
      - "10.0.0.0/8"
    ports:
      - 8080
      - 8911
      - 3000
      - 4000
    protocols:
      - "http"
      - "websocket"
      - "sse"
```

### Automated Discovery

```bash
# Start network scan
curl -X POST http://localhost:8912/api/v1/scan \
  -H "Content-Type: application/json" \
  -d '{
    "targetNetworks": ["192.168.1.0/24"],
    "ports": [8080, 8911],
    "timeout": "30s",
    "protocols": ["http"]
  }'

# Check scan status
curl http://localhost:8912/api/v1/scan/scan-123

# List discovered servers
curl http://localhost:8912/api/v1/servers
```

## Plugins Configuration

### Plugin Registration

```bash
# Register a plugin
curl -X POST http://localhost:8914/api/v1/plugins/register \
  -H "Content-Type: application/json" \
  -d '{
    "name": "weather-plugin",
    "version": "1.0.0",
    "type": "mcp-server",
    "endpoint": {
      "type": "http",
      "url": "http://weather-plugin:8080"
    },
    "capabilities": {
      "tools": ["get_weather", "forecast"]
    }
  }'
```

### Plugin Health Monitoring

```yaml
plugins:
  health:
    enabled: true
    interval: 30s
    timeout: 5s
    failureThreshold: 3
    recovery:
      enabled: true
      strategy: "restart"
      maxAttempts: 3
```

## Monitoring and Observability

### Health Checks

```bash
# Overall system health
curl http://localhost:8911/health

# Individual service health
curl http://localhost:8080/health    # Proxy
curl http://localhost:8913/health    # Registry
curl http://localhost:8912/health    # Discovery
curl http://localhost:8914/health    # Plugins
```

### Metrics Collection

```yaml
monitoring:
  enabled: true
  metrics:
    enabled: true
    path: "/metrics"
    port: 9090
  logging:
    level: "info"
    format: "json"
    outputs:
      - "stdout"
      - "file:/var/log/suse-ai-up.log"
```

### Prometheus Integration

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'suse-ai-up'
    static_configs:
      - targets: ['proxy:9090', 'registry:9090', 'discovery:9090', 'plugins:9090']
```

## Production Deployment

### Kubernetes with Helm

```yaml
# values.yaml
proxy:
  enabled: true
  port: 8080
  tlsPort: 38080
  auth:
    mode: "oauth"
    oauth:
      provider: "azure"
      clientId: "your-client-id"
      tenantId: "your-tenant-id"

registry:
  enabled: true
  port: 8913
  tlsPort: 38913
  sync:
    official: true
    docker: true

discovery:
  enabled: true
  port: 8912
  tlsPort: 38912
  scan:
    enabled: true
    interval: 300

plugins:
  enabled: true
  port: 8914
  tlsPort: 38914
  health:
    enabled: true

ingress:
  enabled: true
  className: "nginx"
  hosts:
    - host: proxy.example.com
      paths:
        - path: /
          pathType: Prefix
    - host: docs.example.com
      paths:
        - path: /
          pathType: Prefix
```

### High Availability Setup

```yaml
# Multiple replicas for HA
proxy:
  replicaCount: 3
  autoscaling:
    enabled: true
    minReplicas: 2
    maxReplicas: 10

registry:
  replicaCount: 2
  persistence:
    enabled: true
    size: 10Gi

discovery:
  replicaCount: 1  # Single instance for scanning

plugins:
  replicaCount: 2
  persistence:
    enabled: true
    size: 5Gi
```

## Networking Configuration

### Service Mesh (Istio)

```yaml
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: suse-ai-up-gateway
spec:
  hosts:
    - proxy.example.com
  gateways:
    - suse-ai-up-gateway
  http:
    - match:
        - uri:
            prefix: /
      route:
        - destination:
            host: proxy
            port:
              number: 8080
---
apiVersion: networking.istio.io/v1beta1
kind: Gateway
metadata:
  name: suse-ai-up-gateway
spec:
  selector:
    istio: ingressgateway
  servers:
    - port:
        number: 80
        name: http
        protocol: HTTP
      hosts:
        - proxy.example.com
```

### Load Balancing

```yaml
# Nginx configuration
upstream suse_ai_up_proxy {
    server proxy-1:8080;
    server proxy-2:8080;
    server proxy-3:8080;
}

server {
    listen 80;
    server_name proxy.example.com;

    location / {
        proxy_pass http://suse_ai_up_proxy;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## Security Configuration

### TLS Configuration

```yaml
tls:
  enabled: true
  autoGenerate: false
  certificate:
    secretName: "suse-ai-up-tls"
  issuer:
    name: "letsencrypt-prod"
    kind: "ClusterIssuer"
```

### Network Policies

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: suse-ai-up-network-policy
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: suse-ai-up
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: ingress-nginx
      ports:
        - protocol: TCP
          port: 8080
        - protocol: TCP
          port: 38080
  egress:
    - to: []
      ports:
        - protocol: TCP
          port: 53  # DNS
        - protocol: TCP
          port: 443 # HTTPS
```

## Backup and Recovery

### Data Backup

```bash
# Backup registry data
curl http://localhost:8913/api/v1/registry/browse > registry_backup.json

# Backup plugin configurations
curl http://localhost:8914/api/v1/plugins > plugins_backup.json

# Backup scan results
curl http://localhost:8912/api/v1/servers > discovery_backup.json
```

### Disaster Recovery

```yaml
backup:
  enabled: true
  schedule: "0 2 * * *"  # Daily at 2 AM
  retention: 30
  storage:
    type: "s3"
    bucket: "suse-ai-up-backups"
    region: "us-west-2"
```

## Troubleshooting

### Service Dependencies

```bash
# Check service connectivity
kubectl exec -it deployment/suse-ai-up-proxy -- curl http://suse-ai-up-registry:8913/health
kubectl exec -it deployment/suse-ai-up-discovery -- curl http://suse-ai-up-registry:8913/health
kubectl exec -it deployment/suse-ai-up-plugins -- curl http://suse-ai-up-registry:8913/health
```

### Log Aggregation

```yaml
logging:
  enabled: true
  fluentd:
    enabled: true
    config: |
      <source>
        @type tail
        path /var/log/containers/*suse-ai-up*.log
        pos_file /var/log/fluentd-containers.log.pos
        tag kubernetes.*
        read_from_head true
      </source>
```

### Performance Monitoring

```bash
# Check resource usage
kubectl top pods -l app.kubernetes.io/name=suse-ai-up

# Check service metrics
curl http://localhost:8080/metrics
curl http://localhost:8913/metrics
```

## Scaling Considerations

### Horizontal Scaling

```yaml
# Scale proxy service
kubectl scale deployment suse-ai-up-proxy --replicas=5

# Scale registry service
kubectl scale deployment suse-ai-up-registry --replicas=3
```

### Vertical Scaling

```yaml
proxy:
  resources:
    requests:
      cpu: 500m
      memory: 1Gi
    limits:
      cpu: 2000m
      memory: 4Gi

registry:
  resources:
    requests:
      cpu: 200m
      memory: 512Mi
    limits:
      cpu: 1000m
      memory: 2Gi
```

## Migration from Basic Setup

### Gradual Migration

1. **Deploy Registry Service**
   ```bash
   helm upgrade --install suse-ai-up-registry ./charts/suse-ai-up \
     --set proxy.enabled=false \
     --set discovery.enabled=false \
     --set plugins.enabled=false
   ```

2. **Add Discovery Service**
   ```bash
   helm upgrade suse-ai-up-registry ./charts/suse-ai-up \
     --set discovery.enabled=true
   ```

3. **Enable Plugins Service**
   ```bash
   helm upgrade suse-ai-up-registry ./charts/suse-ai-up \
     --set plugins.enabled=true
   ```

4. **Enable Proxy Service**
   ```bash
   helm upgrade suse-ai-up-registry ./charts/suse-ai-up \
     --set proxy.enabled=true
   ```

This full stack example provides a production-ready deployment of the SUSE AI Universal Proxy with all services properly configured and integrated.</content>
<parameter name="filePath">examples/full-stack/README.md