# SUSE AI Universal Proxy Quickstart

This guide takes you from zero to hero with SUSE AI Universal Proxy, enabling MCP server management, discovery, and proxying in minutes.

## Prerequisites

- **System**: Linux, macOS, or Windows with WSL2
- **Memory**: Minimum 4GB RAM, 8GB recommended
- **Tools**: curl, kubectl, helm
- **Network**: Internet access for downloads

## Setup Kubernetes

Install a Kubernetes distribution. Example with k3s:

```bash
curl -sfL https://get.k3s.io | sh -
```

Other distributions like minikube, kind, or managed services (EKS, AKS, GKE) may be used.

[Full installation guide](https://docs.k3s.io/quick-start#install-script)

Verify installation:
```bash
kubectl get nodes
```

## Setup Rancher

Install Rancher on your Kubernetes cluster for management UI.

[Installation guide](https://ranchermanager.docs.rancher.com/getting-started/installation-and-upgrade/install-upgrade-on-a-kubernetes-cluster)

Access Rancher UI and select your cluster for deployments.

## Setup SUSE AI Universal Proxy (Helm installation)

1. Open a local terminal 
2. Clone the repository: `https://github.com/suse/suse-ai-up` (branch: main)
3. enter in the folder `suse-ai-up`
4. In values.yaml, set:
   - `service.type: LoadBalancer`
   - `auth.method: development` (for no auth)

Install using the helm chart:

```bash
helm install suse-ai-up ./charts/suse-ai-up
```
5. Wait for the installation to be completed

Get Service IP
```bash
kubectl get svc suse-ai-up-service -n suse-ai-up -o jsonpath='{.status.loadBalancer.ingress[0].ip}'
```

## (Alternative) Setup SUSE AI Universal Proxy (in Rancher)

1. In Rancher, add repository: `https://github.com/suse/suse-ai-up` (branch: main)
2. Go to Apps → Charts
3. Find and install "SUSE AI Universal Proxy"
4. Click Install and wait for completion


## Setup SUSE AI Universal Proxy UI

1. In Rancher, add repository: `https://github.com/suse/suse-ai-up-ext`(branch: v0.1.0)
2. Go to Extensions
3. Find and install "SUSE AI Universal Proxy"

## Verify Installation and Access Swagger Docs

Access API documentation:
```
http://{IP ADDRESS}:8911/docs/index.html
```

Check service health:
```bash
curl http://{IP ADDRESS}:8911/health
```

## API Examples (Dev Mode - No Authentication)

### Check Registry

Browse available MCP servers:
```bash
curl -X GET "http://{IP ADDRESS}:8911/api/v1/registry/browse"
```

### Deploy an Adapter

Create an adapter for a sample MCP server (uyuni):
```bash
curl -X POST "http://{IP ADDRESS}:8911/api/v1/adapters" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "uyuni-adapter",
    "mcpServerId": "uyuni",
    "environmentVariables": {
      "UYUNI_SERVER": "https://uyuni.example.com",
      "UYUNI_USER": "admin",
      "UYUNI_PASS": "password"
    }
  }'
```

### Discover MCP on Network

Start network scan:
```bash
curl -X POST "http://{IP ADDRESS}:8912/api/v1/discovery/scan"
```

Get scan results:
```bash
curl -X GET "http://{IP ADDRESS}:8912/api/v1/discovery/results"
```

## Next Steps

- Explore full [EXAMPLES.md](examples/EXAMPLES.md) for advanced usage
- Check [README.md](README.md) for architecture details
- Visit [GitHub Issues](https://github.com/suse/suse-ai-up/issues) for support</content>
<parameter name="filePath">/Users/alessandrofesta/Documents/innovation/suse-ai-up/QUICKSTART.md