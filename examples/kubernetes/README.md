# Kubernetes Production Deployment

This example demonstrates a production-ready SUSE AI Universal Proxy deployment on Kubernetes using Helm charts with high availability, monitoring, and security best practices.

## Overview

The Kubernetes deployment includes:
- **High Availability**: Multi-replica deployments with rolling updates
- **Security**: TLS encryption, network policies, and RBAC
- **Monitoring**: Prometheus metrics and health checks
- **Storage**: Persistent volumes for data persistence
- **Ingress**: Load balancer configuration with TLS termination
- **Backup**: Automated backup and recovery procedures

## Prerequisites

### Kubernetes Cluster

- Kubernetes 1.24+
- Helm 3.8+
- Cert-Manager (for TLS certificates)
- Prometheus Operator (for monitoring)
- Ingress Controller (NGINX or Traefik)

### Install Prerequisites

```bash
# Install cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

# Install Prometheus Operator
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm install prometheus prometheus-community/kube-prometheus-stack

# Install NGINX Ingress Controller
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm install nginx ingress-nginx/ingress-nginx
```

## Quick Start

### Deploy with Default Configuration

```bash
# Add Helm repository
helm repo add suse-ai-up https://charts.suse.ai
helm repo update

# Install SUSE AI Universal Proxy
helm install suse-ai-up suse-ai-up/suse-ai-up \
  --namespace suse-ai-up \
  --create-namespace
```

### Access the Services

```bash
# Get service endpoints
kubectl get ingress -n suse-ai-up

# Access Swagger documentation
open https://proxy.example.com/docs

# Check service health
curl https://proxy.example.com/health
```

## Configuration

### values.yaml

```yaml
# Global configuration
global:
  imageRegistry: "suse"
  imagePullSecrets:
    - name: "registry-secret"
  storageClass: "fast-ssd"

# Proxy service configuration
proxy:
  enabled: true
  replicaCount: 3

  image:
    repository: suse/suse-ai-up
    tag: "latest"
    pullPolicy: IfNotPresent

  service:
    type: ClusterIP
    port: 8080
    tlsPort: 38080
    annotations:
      service.beta.kubernetes.io/aws-load-balancer-type: nlb

  resources:
    requests:
      cpu: 500m
      memory: 1Gi
    limits:
      cpu: 2000m
      memory: 4Gi

  autoscaling:
    enabled: true
    minReplicas: 2
    maxReplicas: 10
    targetCPUUtilizationPercentage: 70
    targetMemoryUtilizationPercentage: 80

  config:
    auth:
      mode: "oauth"
      oauth:
        provider: "azure"
        clientId: "your-client-id"
        tenantId: "your-tenant-id"
    tls:
      enabled: true
      autoGenerate: false

# Registry service configuration
registry:
  enabled: true
  replicaCount: 2

  persistence:
    enabled: true
    size: 10Gi
    accessMode: ReadWriteOnce

  config:
    sync:
      official: true
      docker: true
      interval: 24h

# Discovery service configuration
discovery:
  enabled: true
  replicaCount: 1

  config:
    scan:
      enabled: true
      interval: 300
      networks:
        - "10.0.0.0/8"
        - "192.168.0.0/16"

# Plugins service configuration
plugins:
  enabled: true
  replicaCount: 2

  persistence:
    enabled: true
    size: 5Gi

  config:
    health:
      enabled: true
      interval: 30s

# Ingress configuration
ingress:
  enabled: true
  className: "nginx"
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    nginx.ingress.kubernetes.io/force-ssl-redirect: "true"
  hosts:
    - host: proxy.example.com
      paths:
        - path: /
          pathType: Prefix
    - host: docs.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: suse-ai-up-tls
      hosts:
        - proxy.example.com
        - docs.example.com

# Certificate issuer
certIssuer:
  enabled: true
  name: letsencrypt-prod
  server: https://acme-v02.api.letsencrypt.org/directory
  email: admin@example.com

# Monitoring configuration
monitoring:
  enabled: true
  serviceMonitor:
    enabled: true
    namespace: monitoring
    interval: 30s
  prometheusRule:
    enabled: true
    rules:
      - alert: SUSEAIUPDown
        expr: up{job="suse-ai-up"} == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "SUSE AI Universal Proxy is down"

# Backup configuration
backup:
  enabled: true
  schedule: "0 2 * * *"
  image:
    repository: suse/suse-ai-up-backup
    tag: "latest"
  config:
    s3:
      bucket: "suse-ai-up-backups"
      region: "us-west-2"
      endpoint: ""
    retention:
      days: 30

# Security configuration
security:
  networkPolicy:
    enabled: true
  podSecurityContext:
    runAsNonRoot: true
    runAsUser: 1000
    fsGroup: 2000
  securityContext:
    allowPrivilegeEscalation: false
    readOnlyRootFilesystem: true
    capabilities:
      drop:
        - ALL
```

## Service Architecture

### Pod Structure

```
suse-ai-up-proxy-7f8b9c4d5f-abc12 (Proxy Pod)
├── proxy-container (Main)
├── oauth-sidecar (OAuth proxy)
└── metrics-exporter (Metrics)

suse-ai-up-registry-6d7c8b3a2e-def34 (Registry Pod)
├── registry-container (Main)
├── sync-manager (Sync)
└── backup-agent (Backup)

suse-ai-up-discovery-5e6f7a2b1c-ghi56 (Discovery Pod)
├── discovery-container (Main)
└── scanner (Network scanner)

suse-ai-up-plugins-4f5g6h3c2d-jkl78 (Plugins Pod)
├── plugins-container (Main)
├── health-monitor (Health)
└── plugin-loader (Loader)
```

### Service Mesh

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
            prefix: /api/v1/registry
      route:
        - destination:
            host: suse-ai-up-registry
    - match:
        - uri:
            prefix: /api/v1/discovery
      route:
        - destination:
            host: suse-ai-up-discovery
    - match:
        - uri:
            prefix: /api/v1/plugins
      route:
        - destination:
            host: suse-ai-up-plugins
    - route:
        - destination:
            host: suse-ai-up-proxy
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
        number: 443
        name: https
        protocol: HTTPS
      tls:
        mode: SIMPLE
        credentialName: suse-ai-up-tls
      hosts:
        - proxy.example.com
        - docs.example.com
```

## High Availability

### Multi-Zone Deployment

```yaml
# Node affinity for multi-zone HA
affinity:
  nodeAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        preference:
          matchExpressions:
            - key: topology.kubernetes.io/zone
              operator: In
              values:
                - us-west-2a
                - us-west-2b
                - us-west-2c

# Pod anti-affinity to spread across nodes
podAntiAffinity:
  preferredDuringSchedulingIgnoredDuringExecution:
    - weight: 100
      podAffinityTerm:
        labelSelector:
          matchLabels:
            app.kubernetes.io/name: suse-ai-up
            app.kubernetes.io/component: proxy
        topologyKey: kubernetes.io/hostname
```

### Rolling Updates

```yaml
strategy:
  type: RollingUpdate
  rollingUpdate:
    maxUnavailable: 1
    maxSurge: 1

# Blue-green deployment
blueGreen:
  enabled: false
  activeService: blue
  previewService: green
```

## Security

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
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: suse-ai-up
      ports:
        - protocol: TCP
          port: 8080
        - protocol: TCP
          port: 8913
        - protocol: TCP
          port: 8912
        - protocol: TCP
          port: 8914
  egress:
    - to:
        - ipBlock:
            cidr: 0.0.0.0/0
      ports:
        - protocol: TCP
          port: 443  # HTTPS
        - protocol: TCP
          port: 80   # HTTP
        - protocol: TCP
          port: 53   # DNS
```

### RBAC Configuration

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: suse-ai-up-role
rules:
  - apiGroups: [""]
    resources: ["pods", "services", "configmaps", "secrets"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["apps"]
    resources: ["deployments", "replicasets"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: suse-ai-up-rolebinding
subjects:
  - kind: ServiceAccount
    name: suse-ai-up-sa
roleRef:
  kind: Role
  name: suse-ai-up-role
  apiGroup: rbac.authorization.k8s.io
```

### TLS Configuration

```yaml
# Certificate management with cert-manager
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: admin@example.com
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
      - http01:
          ingress:
            class: nginx
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: suse-ai-up-tls
spec:
  secretName: suse-ai-up-tls
  issuerRef:
    name: letsencrypt-prod
    kind: ClusterIssuer
  dnsNames:
    - proxy.example.com
    - docs.example.com
    - api.example.com
```

## Monitoring

### Prometheus Metrics

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: suse-ai-up-servicemonitor
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: suse-ai-up
  endpoints:
    - port: metrics
      interval: 30s
      path: /metrics
  namespaceSelector:
    matchNames:
      - suse-ai-up
```

### Grafana Dashboards

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: suse-ai-up-grafana-dashboard
  labels:
    grafana_dashboard: "1"
data:
  suse-ai-up-dashboard.json: |
    {
      "dashboard": {
        "title": "SUSE AI Universal Proxy",
        "panels": [
          {
            "title": "Request Rate",
            "type": "graph",
            "targets": [
              {
                "expr": "rate(mcp_proxy_requests_total[5m])",
                "legendFormat": "{{method}} {{status}}"
              }
            ]
          }
        ]
      }
    }
```

### Alerting Rules

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: suse-ai-up-alerts
spec:
  groups:
    - name: suse-ai-up
      rules:
        - alert: SUSEAIUPHighErrorRate
          expr: rate(mcp_proxy_requests_total{status=~"5.."}[5m]) / rate(mcp_proxy_requests_total[5m]) > 0.05
          for: 5m
          labels:
            severity: warning
          annotations:
            summary: "High error rate in SUSE AI Universal Proxy"
        - alert: SUSEAIUPDown
          expr: up{job="suse-ai-up"} == 0
          for: 5m
          labels:
            severity: critical
          annotations:
            summary: "SUSE AI Universal Proxy service is down"
```

## Storage

### Persistent Volumes

```yaml
# Registry storage
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: suse-ai-up-registry-storage
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: fast-ssd
  resources:
    requests:
      storage: 10Gi

# Plugins storage
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: suse-ai-up-plugins-storage
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: fast-ssd
  resources:
    requests:
      storage: 5Gi
```

### Backup Strategy

```yaml
apiVersion: batch/v1beta1
kind: CronJob
metadata:
  name: suse-ai-up-backup
spec:
  schedule: "0 2 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          containers:
            - name: backup
              image: suse/suse-ai-up-backup:latest
              env:
                - name: S3_BUCKET
                  value: "suse-ai-up-backups"
                - name: S3_REGION
                  value: "us-west-2"
              command:
                - /bin/bash
                - -c
                - |
                  # Backup registry data
                  curl http://suse-ai-up-registry:8913/api/v1/registry/browse > /tmp/registry.json
                  aws s3 cp /tmp/registry.json s3://$S3_BUCKET/registry-$(date +%Y%m%d).json

                  # Backup plugin configurations
                  curl http://suse-ai-up-plugins:8914/api/v1/plugins > /tmp/plugins.json
                  aws s3 cp /tmp/plugins.json s3://$S3_BUCKET/plugins-$(date +%Y%m%d).json
          restartPolicy: OnFailure
```

## Scaling

### Horizontal Pod Autoscaling

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: suse-ai-up-proxy-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: suse-ai-up-proxy
  minReplicas: 2
  maxReplicas: 10
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: 80
```

### Vertical Scaling

```yaml
# Update resource limits
kubectl patch deployment suse-ai-up-proxy -p '{
  "spec": {
    "template": {
      "spec": {
        "containers": [{
          "name": "proxy",
          "resources": {
            "requests": {"cpu": "1", "memory": "2Gi"},
            "limits": {"cpu": "2", "memory": "4Gi"}
          }
        }]
      }
    }
  }
}'
```

## Troubleshooting

### Common Issues

**Pod Startup Failures**
```bash
# Check pod status
kubectl get pods -n suse-ai-up

# Check pod logs
kubectl logs -f deployment/suse-ai-up-proxy -n suse-ai-up

# Check events
kubectl get events -n suse-ai-up --sort-by=.metadata.creationTimestamp
```

**Service Connectivity Issues**
```bash
# Check service endpoints
kubectl get endpoints -n suse-ai-up

# Test service connectivity
kubectl exec -it deployment/suse-ai-up-proxy -n suse-ai-up -- curl http://suse-ai-up-registry:8913/health
```

**Ingress Issues**
```bash
# Check ingress status
kubectl describe ingress suse-ai-up-ingress -n suse-ai-up

# Check ingress controller logs
kubectl logs -n ingress-nginx deployment/ingress-nginx-controller
```

### Debug Commands

```bash
# Port forward for local testing
kubectl port-forward -n suse-ai-up svc/suse-ai-up-proxy 8080:8080

# Execute into pod for debugging
kubectl exec -it -n suse-ai-up deployment/suse-ai-up-proxy -- /bin/bash

# Check resource usage
kubectl top pods -n suse-ai-up

# Check network policies
kubectl describe networkpolicy -n suse-ai-up
```

## Backup and Recovery

### Manual Backup

```bash
# Create backup namespace
kubectl create namespace suse-ai-up-backup

# Run backup job
kubectl create job manual-backup --from=cronjob/suse-ai-up-backup -n suse-ai-up-backup

# Check backup status
kubectl logs job/manual-backup -n suse-ai-up-backup
```

### Disaster Recovery

```bash
# Scale down services
kubectl scale deployment suse-ai-up-proxy --replicas=0 -n suse-ai-up
kubectl scale deployment suse-ai-up-registry --replicas=0 -n suse-ai-up

# Restore from backup
kubectl create job restore-backup -n suse-ai-up --image=suse/suse-ai-up-backup \
  --env="RESTORE_DATE=2025-12-04" \
  -- /bin/bash -c "restore-from-s3.sh"

# Scale up services
kubectl scale deployment suse-ai-up-registry --replicas=2 -n suse-ai-up
kubectl scale deployment suse-ai-up-proxy --replicas=3 -n suse-ai-up
```

## Performance Tuning

### Resource Optimization

```yaml
proxy:
  resources:
    requests:
      cpu: 500m
      memory: 1Gi
    limits:
      cpu: 2000m
      memory: 4Gi
  env:
    - name: GOMAXPROCS
      value: "2"
    - name: GOGC
      value: "100"

registry:
  resources:
    requests:
      cpu: 200m
      memory: 512Mi
    limits:
      cpu: 1000m
      memory: 2Gi
  config:
    cache:
      enabled: true
      size: 10000
      ttl: 3600
```

### Database Optimization

```yaml
registry:
  config:
    database:
      maxConnections: 50
      connectionTimeout: 30s
      queryTimeout: 10s
      poolSize: 10
```

## CI/CD Integration

### GitOps with Flux

```yaml
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: GitRepository
metadata:
  name: suse-ai-up
spec:
  interval: 1m
  url: https://github.com/your-org/suse-ai-up-deploy
  ref:
    branch: main
---
apiVersion: kustomize.toolkit.fluxcd.io/v1beta2
kind: Kustomization
metadata:
  name: suse-ai-up
spec:
  interval: 5m
  path: "./clusters/production"
  prune: true
  sourceRef:
    kind: GitRepository
    name: suse-ai-up
```

### Automated Testing

```yaml
# Helm test
apiVersion: v1
kind: Pod
metadata:
  name: "{{ include "suse-ai-up.fullname" . }}-test"
  annotations:
    "helm.sh/hook": test
spec:
  containers:
    - name: test
      image: suse/suse-ai-up:latest
      command:
        - /bin/bash
        - -c
        - |
          # Wait for services to be ready
          for i in {1..30}; do
            if curl -f http://suse-ai-up-proxy:8080/health; then
              echo "Services are healthy"
              exit 0
            fi
            sleep 10
          done
          echo "Services failed to become healthy"
          exit 1
  restartPolicy: Never
```

This Kubernetes example provides a comprehensive production deployment of the SUSE AI Universal Proxy with enterprise-grade features including high availability, security, monitoring, and automated operations.</content>
<parameter name="filePath">examples/kubernetes/README.md