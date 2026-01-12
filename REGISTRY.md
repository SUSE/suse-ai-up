# MCP Registry Documentation

This document explains how to create and manage entries in the MCP (Model Context Protocol) registry for the SUSE AI Universal Proxy.

## Overview

The MCP registry (`config/mcp_registry.yaml`) contains metadata about available MCP servers that can be deployed as sidecars. Each entry defines how to run an MCP server with support for three deployment types:

- **docker**: Traditional Docker container deployments
- **python**: Python-based MCP servers with automatic uv setup
- **npx**: Node.js-based MCP servers using npm packages

## Registry Entry Structure

Each registry entry follows this YAML structure:

```yaml
- name: <server-name>
  image: <docker-image>  # Required for docker type, optional for others
  type: server
  meta:
    category: <category>
    tags:
      - <tag1>
      - <tag2>
    sidecarConfig:
      commandType: <docker|python|npx>
      command: <command-string>
      port: <port-number>
      source: <source-identifier>
      lastUpdated: <timestamp>
  about:
    icon: <icon-url>
    title: <display-title>
    description: <description-text>
  source:
    commit: <commit-hash>
    project: <repository-url>  # Required for python type
  config:
    description: <config-description>
    secrets:
      - env: <env-var-name>
        example: <example-value>
        name: <config-key>
    parameters:
      properties:
        <param-name>:
          type: <param-type>
          default: <default-value>
      required:
        - <required-param>
    env:
      - name: <env-var-name>
        value: <default-value>
```

## Command Types

### Docker Command Type

For traditional Docker container deployments.

```yaml
- name: my-docker-server
  image: myorg/my-mcp-server:latest
  type: server
  meta:
    category: productivity
    tags:
      - docker
      - productivity
    sidecarConfig:
      commandType: docker
      command: "docker run -i --rm -e API_KEY=your_key myorg/my-mcp-server:latest"
      port: 8000
      source: manual-config
      lastUpdated: '2025-12-12T10:00:00Z'
  about:
    icon: https://example.com/icon.png
    title: My Docker MCP Server
    description: A Docker-based MCP server for productivity tasks
  source:
    commit: abc123def456
    project: https://github.com/myorg/my-mcp-server
  config:
    description: Configure the Docker MCP server
    secrets:
      - env: API_KEY
        example: your_api_key_here
        name: api_key
```

**Notes:**
- The `image` field is required for docker deployments
- The `command` should be a complete docker run command
- Environment variables can be templated using the adapter's environment variables

### Python Command Type

For Python-based MCP servers that require repository cloning and uv setup.

```yaml
- name: bugzilla
  image: kskarthik/mcp-bugzilla:latest  # Optional, for reference
  type: server
  meta:
    category: issue-tracking
    tags:
      - bugzilla
      - issue-tracking
      - python
    sidecarConfig:
      commandType: python
      command: "uv run mcp-bugzilla --bugzilla-server https://bugzilla.example.com --host 127.0.0.1 --port 8000"
      port: 8000
      source: manual-config
      lastUpdated: '2025-12-11T16:30:00Z'
  about:
    icon: https://apps.rancher.io/logos/suse-ai-deployer.png
    title: SUSE Bugzilla
    description: MCP server for Bugzilla issue tracking
  source:
    commit: 040bc4b80f18e4a60deae1aa9f0dcf5c5b0bb0bf
    project: https://github.com/openSUSE/mcp-bugzilla
  config:
    description: Configure Bugzilla server connection
    secrets:
      - env: BUGZILLA_SERVER
        example: https://bugzilla.suse.com
        name: bugzilla.server
      - env: BUGZILLA_APIKEY
        example: your_api_key_here
        name: bugzilla.apikey
```

**Deployment Process:**
1. Installs `uv` package manager via pip
2. Installs `git` via zypper
3. Clones the repository from `source.project`
4. Runs `uv sync` to set up dependencies
5. Executes the specified command

**Notes:**
- The `source.project` field is required for python deployments
- The system automatically handles the complete setup process
- Repository URL should be publicly accessible or authentication should be configured

### NPX Command Type

For Node.js-based MCP servers using npm packages.

```yaml
- name: airtable-mcp-server
  image: mcp/airtable-mcp-server  # Optional, for reference
  type: server
  meta:
    category: productivity
    tags:
      - airtable
      - productivity
      - npx
    sidecarConfig:
      commandType: npx
      command: "npx -y @nekzus/npm-sentinel-mcp"
      port: 8000
      source: manual-config
      lastUpdated: '2025-12-18T15:14:30.169082Z'
  about:
    description: Provides AI assistants with direct access to Airtable bases
    icon: https://www.google.com/s2/favicons?domain=airtable.com&sz=64
    title: Airtable
  source:
    branch: master
    commit: e6ab2431b144865e403976d50549dfafd7be7283
    project: https://github.com/domdomegg/airtable-mcp-server
  config:
    description: Configure the connection to Airtable mcp server
    env:
      - example: production
        name: NODE_ENV
        value: '{{airtable-mcp-server.nodeenv}}'
    parameters:
      properties:
        nodeenv:
          type: string
      type: object
    secrets:
      - env: AIRTABLE_API_KEY
        example: patABC123.def456ghi789jkl012mno345pqr678stu901vwx
        name: airtable-mcp-server.api_key
```

**Notes:**
- Uses the BCI Node.js container image (`registry.suse.com/bci/nodejs:22`)
- The `command` should start with `npx -y` for non-interactive installation
- Environment variables are automatically passed to the container

## ConfigMap Management

### Creating a ConfigMap from Local Registry

To create a ConfigMap from the local registry file:

```bash
kubectl create configmap suse-ai-up-registry \
  --from-file=mcp_registry.yaml=config/mcp_registry.yaml \
  -n suseai
```

### Updating an Existing ConfigMap

To update an existing ConfigMap:

```bash
kubectl delete configmap suse-ai-up-registry -n suseai
kubectl create configmap suse-ai-up-registry \
  --from-file=mcp_registry.yaml=config/mcp_registry.yaml \
  -n suseai
```

## Remote Registry Loading

The system supports loading registries from remote URLs, which is useful for:

- Centralized registry management
- Automated updates
- Private repositories

### Configuration

Set the remote registry URL using the `MCP_REGISTRY_URL` environment variable:

```bash
export MCP_REGISTRY_URL=https://example.com/path/to/mcp_registry.yaml
```

Or configure it in your deployment:

```yaml
env:
  - name: MCP_REGISTRY_URL
    value: "https://example.com/path/to/mcp_registry.yaml"
```

### Authentication for Private Repositories

For private Git repositories, configure authentication using one of these methods:

#### 1. Personal Access Token (Recommended)

```bash
export MCP_REGISTRY_URL=https://oauth2:YOUR_TOKEN@github.com/yourorg/private-mcp-registry/main/mcp_registry.yaml
```

Or in the URL:
```
https://oauth2:YOUR_TOKEN@github.com/yourorg/private-mcp-registry/main/mcp_registry.yaml
```

#### 2. SSH Key Authentication

For SSH URLs, ensure the container has access to SSH keys:

```yaml
env:
  - name: GIT_SSH_COMMAND
    value: "ssh -i /path/to/private/key -o StrictHostKeyChecking=no"
```

#### 3. Git Credentials

Configure git credentials in the container:

```bash
git config --global credential.helper store
echo "https://username:password@github.com" > ~/.git-credentials
```

### Registry Loading Process

1. **URL Validation**: Validates the provided URL format
2. **Authentication**: Uses configured credentials for private repos
3. **Download**: Fetches the registry YAML from the remote URL
4. **Parsing**: Parses and validates the YAML structure
5. **Registration**: Registers all MCP servers in the system

### Timeout Configuration

Configure registry download timeout:

```yaml
env:
  - name: MCP_REGISTRY_TIMEOUT
    value: "30s"  # Default timeout
```

## Best Practices

### Registry Entry Guidelines

1. **Naming**: Use descriptive, unique names without spaces
2. **Categories**: Choose appropriate categories (productivity, database, devops, etc.)
3. **Tags**: Add relevant tags for discoverability
4. **Documentation**: Provide clear descriptions and configuration examples
5. **Validation**: Test registry entries before deployment

### Command Type Selection

- **Use `docker`** for:
  - Complex multi-stage builds
  - Pre-built containers
  - Non-Python/Node.js applications

- **Use `python`** for:
  - Python-based MCP servers
  - Projects using uv for dependency management
  - Repositories requiring custom setup

- **Use `npx`** for:
  - NPM-published MCP servers
  - Simple Node.js applications
  - Quick prototyping

### Security Considerations

1. **Secrets Management**: Never commit secrets to registry files
2. **Private Repositories**: Use appropriate authentication methods
3. **Access Control**: Configure route assignments for sensitive servers
4. **Image Security**: Use trusted base images (preferably BCI)

### Maintenance

1. **Version Tracking**: Keep commit hashes updated
2. **Regular Updates**: Check for upstream updates regularly
3. **Testing**: Validate registry changes before deployment
4. **Documentation**: Update this document when adding new features

## Troubleshooting

### Common Issues

#### Python Setup Failures
- **Symptom**: Pod crashes during uv sync
- **Solution**: Check repository URL accessibility and authentication
- **Debug**: Check pod logs for git clone or uv sync errors

#### NPX Installation Issues
- **Symptom**: Package installation fails
- **Solution**: Verify package name and network connectivity
- **Debug**: Check for `npx -y` prefix in command

#### Docker Command Issues
- **Symptom**: Container fails to start
- **Solution**: Validate docker run command syntax
- **Debug**: Check environment variable templating

#### Authentication Problems
- **Symptom**: Registry download fails with 401/403
- **Solution**: Verify authentication credentials and URL format
- **Debug**: Test URL access manually with configured credentials

### Debug Commands

Check registry loading:
```bash
kubectl logs -n suseai deployment/suse-ai-up | grep -i registry
```

Check sidecar deployment logs:
```bash
kubectl logs -n suse-ai-up-mcp <pod-name>
```

Validate registry format:
```bash
kubectl exec -n suseai <pod-name> -- cat /home/mcpuser/config/mcp_registry.yaml | head -50
```

## Examples

See the existing `config/mcp_registry.yaml` file for comprehensive examples of all command types and configurations.</content>
<parameter name="filePath">REGISTRY.md