# Minimal SUSE AI Universal Proxy Deployment

This directory contains a minimal Kubernetes deployment for the SUSE AI Universal Proxy service.

## Files

- `deployment.yaml` - Kubernetes Deployment with 1 replica
- `service.yaml` - ClusterIP Service exposing port 8911

## Usage

### Deploy to Kubernetes

```bash
# Apply the deployment and service
kubectl apply -f deployment/minimal/

# Check the deployment status
kubectl get deployments
kubectl get services

# Check pod status
kubectl get pods -l app=suse-ai-up

# View logs
kubectl logs -l app=suse-ai-up
```

### Access the Service

```bash
# Port forward to access locally
kubectl port-forward svc/suse-ai-up-minimal 8911:8911

# Access the API
curl http://localhost:8911/health
```

### Clean Up

```bash
# Remove the deployment and service
kubectl delete -f deployment/minimal/
```

## Configuration

The minimal deployment includes:

- **Image**: `ghcr.io/alessandro-festa/suse-ai-up:latest`
- **Port**: 8911
- **Service Type**: ClusterIP
- **Resources**: 256Mi memory request, 512Mi limit; 100m CPU request, 500m limit

## Prerequisites

- Kubernetes cluster access
- Image `ghcr.io/alessandro-festa/suse-ai-up:latest` must be available in GHCR
- Appropriate permissions to pull from GitHub Container Registry</content>
</xai:function_call name="list">
<parameter name="path">deployment/minimal