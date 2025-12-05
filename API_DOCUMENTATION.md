# SUSE AI Universal Proxy - Vue.js Rancher Extension API Documentation

## Overview

The SUSE AI Universal Proxy provides a comprehensive platform for managing Model Context Protocol (MCP) servers, including:

- **Registry**: Curated collection of remote MCP servers from mcpservers.org
- **Discovery**: Network scanning for local MCP server discovery
- **Proxy**: MCP protocol proxying with CORS support
- **Adapters**: User-managed connections to MCP servers with capability discovery

This documentation is designed for Vue.js developers building a Rancher extension UI.

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Registry      │    │   Discovery     │    │     Proxy       │
│   Service       │    │   Service       │    │   Service       │
│                 │    │                 │    │                 │
│ • Static server │    │ • Network scan  │    │ • MCP protocol  │
│   database      │    │ • Server detect │    │ • CORS enabled  │
│ • Manual reload │    │ • Scan status   │    │ • Swagger docs  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                    ┌─────────────────┐
                    │  Adapters       │
                    │  Management     │
                    │                 │
                    │ • User-scoped   │
                    │ • Capability    │
                    │   discovery     │
                    │ • Env vars      │
                    │ • MCP client    │
                    │   config        │
                    └─────────────────┘
```

## Authentication

All API requests require authentication headers:

```javascript
const authHeaders = {
  'X-API-Key': 'your-api-key',
  'X-User-ID': 'user-id'  // Optional, defaults to 'default-user'
}
```

### Rancher Integration

For Rancher extensions, use Rancher authentication tokens:

```javascript
// composables/useRancherAuth.js
import { useAuth } from '@rancher/core'

export function useRancherAuth() {
  const { getToken } = useAuth()

  const getAuthHeaders = () => ({
    'Authorization': `Bearer ${getToken()}`,
    'X-API-Key': 'rancher-managed-key'
  })

  return { getAuthHeaders }
}
```

## API Reference

### Registry Service

#### Browse MCP Servers

**Endpoint**: `GET /api/v1/registry/browse`

**Query Parameters**:
- `q` (string): Search query
- `category` (string): Filter by category (development, productivity, etc.)

**Response**:
```json
[
  {
    "id": "github",
    "name": "GitHub",
    "description": "GitHub's official MCP Server",
    "packages": [
      {
        "registryType": "remote-http",
        "identifier": "api.githubcopilot.com",
        "transport": { "type": "http" }
      }
    ],
    "_meta": {
      "source": "mcpservers.org",
      "userAuthRequired": true,
      "authType": "oauth",
      "category": "development",
      "tags": ["git", "repository", "issues"]
    }
  }
]
```

**Vue.js Example**:
```javascript
// composables/useRegistry.js
import { ref } from 'vue'
import { useApi } from './useApi'

export function useRegistry() {
  const { apiClient } = useApi()
  const servers = ref([])
  const loading = ref(false)

  const browseServers = async (params = {}) => {
    loading.value = true
    try {
      const response = await apiClient.get('/api/v1/registry/browse', { params })
      servers.value = response.data
      return response.data
    } finally {
      loading.value = false
    }
  }

  return {
    servers: readonly(servers),
    loading: readonly(loading),
    browseServers
  }
}
```

```vue
<!-- components/registry/ServerBrowser.vue -->
<template>
  <div class="server-browser">
    <div class="search-bar">
      <input
        v-model="searchQuery"
        placeholder="Search MCP servers..."
        @input="debouncedSearch"
      />
      <select v-model="selectedCategory">
        <option value="">All Categories</option>
        <option value="development">Development</option>
        <option value="productivity">Productivity</option>
        <option value="monitoring">Monitoring</option>
      </select>
    </div>

    <div v-if="loading" class="loading-spinner">
      Loading servers...
    </div>

    <div v-else-if="error" class="error-message">
      {{ error }}
    </div>

    <div v-else class="server-grid">
      <ServerCard
        v-for="server in filteredServers"
        :key="server.id"
        :server="server"
        @create-adapter="$emit('create-adapter', server)"
      />
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRegistry } from '@/composables/useRegistry'
import { debounce } from 'lodash-es'
import ServerCard from './ServerCard.vue'

const emit = defineEmits(['create-adapter'])

const { servers, loading, browseServers } = useRegistry()
const searchQuery = ref('')
const selectedCategory = ref('')
const error = ref('')

const filteredServers = computed(() => {
  return servers.value.filter(server => {
    const matchesSearch = !searchQuery.value ||
      server.name.toLowerCase().includes(searchQuery.value.toLowerCase()) ||
      server.description.toLowerCase().includes(searchQuery.value.toLowerCase())

    const matchesCategory = !selectedCategory.value ||
      server._meta?.category === selectedCategory.value

    return matchesSearch && matchesCategory
  })
})

const debouncedSearch = debounce(async () => {
  try {
    await browseServers({
      q: searchQuery.value,
      category: selectedCategory.value
    })
  } catch (err) {
    error.value = err.message
  }
}, 300)

onMounted(() => {
  browseServers()
})
</script>
```

#### Get Server Details

**Endpoint**: `GET /api/v1/registry/{id}`

**Response**: Single server object (same format as browse)

#### Reload Server Registry

**Endpoint**: `POST /api/v1/registry/reload`

**Response**:
```json
{
  "status": "reload_completed",
  "message": "Remote MCP servers reloaded successfully"
}
```

### Discovery Service

#### Start Network Scan

**Endpoint**: `POST /api/v1/scan`

**Request**:
```json
{
  "scanRanges": ["192.168.1.0/24"],
  "ports": ["3000", "4000"],
  "timeout": "30s",
  "maxConcurrent": 10,
  "excludeProxy": true
}
```

**Response**:
```json
{
  "scan_id": "scan-12345",
  "status": "started",
  "message": "Network scan initiated"
}
```

**Vue.js Example**:
```javascript
// composables/useDiscovery.js
export function useDiscovery() {
  const { apiClient } = useApi()
  const scans = ref([])
  const currentScan = ref(null)

  const startScan = async (config) => {
    const response = await apiClient.post('/api/v1/scan', config)
    currentScan.value = {
      id: response.data.scan_id,
      status: 'running',
      config
    }
    return response.data
  }

  const getScanStatus = async (scanId) => {
    const response = await apiClient.get(`/api/v1/scan/${scanId}`)
    return response.data
  }

  const listDiscoveredServers = async () => {
    const response = await apiClient.get('/api/v1/servers')
    return response.data
  }

  return {
    scans: readonly(scans),
    currentScan: readonly(currentScan),
    startScan,
    getScanStatus,
    listDiscoveredServers
  }
}
```

```vue
<!-- components/discovery/ScanManager.vue -->
<template>
  <div class="scan-manager">
    <div class="scan-form">
      <h3>Network Scan Configuration</h3>
      <form @submit.prevent="startScan">
        <div class="form-row">
          <label>Network Ranges</label>
          <input
            v-model="scanConfig.scanRanges"
            placeholder="192.168.1.0/24, 10.0.0.0/8"
            required
          />
        </div>

        <div class="form-row">
          <label>Ports</label>
          <input
            v-model="scanConfig.ports"
            placeholder="3000,4000,8000"
            required
          />
        </div>

        <div class="form-row">
          <label>Timeout</label>
          <input
            v-model="scanConfig.timeout"
            placeholder="30s"
            required
          />
        </div>

        <button type="submit" :disabled="scanning">
          {{ scanning ? 'Scanning...' : 'Start Scan' }}
        </button>
      </form>
    </div>

    <div v-if="currentScan" class="scan-status">
      <h3>Scan Progress</h3>
      <div class="progress-bar">
        <div
          class="progress-fill"
          :style="{ width: progressPercent + '%' }"
        ></div>
      </div>
      <p>Status: {{ currentScan.status }}</p>
      <p>Found: {{ discoveredServers.length }} servers</p>
    </div>

    <div v-if="discoveredServers.length > 0" class="results">
      <h3>Discovered Servers</h3>
      <div class="server-list">
        <div
          v-for="server in discoveredServers"
          :key="server.id"
          class="server-item"
        >
          <div class="server-info">
            <strong>{{ server.address }}:{{ server.port }}</strong>
            <span class="last-seen">Last seen: {{ formatDate(server.lastSeen) }}</span>
          </div>
          <button @click="$emit('register-server', server)">
            Register as Adapter
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useDiscovery } from '@/composables/useDiscovery'

const emit = defineEmits(['register-server'])

const { startScan, getScanStatus, listDiscoveredServers, currentScan } = useDiscovery()
const scanConfig = ref({
  scanRanges: '',
  ports: '3000,4000,8000',
  timeout: '30s',
  maxConcurrent: 10,
  excludeProxy: true
})
const scanning = ref(false)
const discoveredServers = ref([])
const progressPercent = ref(0)

const startScanHandler = async () => {
  try {
    scanning.value = true
    progressPercent.value = 0

    // Parse comma-separated values
    const config = {
      ...scanConfig.value,
      scanRanges: scanConfig.value.scanRanges.split(',').map(s => s.trim()),
      ports: scanConfig.value.ports.split(',').map(s => s.trim())
    }

    await startScan(config)

    // Start polling for status
    pollScanStatus()
  } catch (err) {
    console.error('Scan failed:', err)
  } finally {
    scanning.value = false
  }
}

const pollScanStatus = async () => {
  if (!currentScan.value) return

  try {
    const status = await getScanStatus(currentScan.value.id)
    currentScan.value.status = status.status

    if (status.status === 'completed') {
      discoveredServers.value = await listDiscoveredServers()
      progressPercent.value = 100
    } else if (status.status === 'running') {
      // Estimate progress (this is approximate)
      progressPercent.value = Math.min(progressPercent.value + 10, 90)
      setTimeout(pollScanStatus, 2000)
    }
  } catch (err) {
    console.error('Status check failed:', err)
  }
}

const formatDate = (dateString) => {
  return new Date(dateString).toLocaleString()
}
</script>
```

#### Get Scan Status

**Endpoint**: `GET /api/v1/scan/{id}`

**Response**:
```json
{
  "id": "scan-12345",
  "config": {
    "scanRanges": ["192.168.1.0/24"],
    "ports": ["3000", "4000"],
    "timeout": "30s"
  },
  "startTime": "2025-01-01T10:00:00Z",
  "status": "running|completed|failed",
  "results": [
    {
      "id": "server-1",
      "address": "192.168.1.100",
      "port": 3000,
      "protocol": "http",
      "discoveredAt": "2025-01-01T10:00:05Z",
      "lastSeen": "2025-01-01T10:00:05Z"
    }
  ]
}
```

#### List Discovered Servers

**Endpoint**: `GET /api/v1/servers`

**Response**: Array of discovered server objects

### Proxy Service

#### MCP Protocol Endpoint

**Endpoint**: `POST /mcp`

**Request**: MCP JSON-RPC protocol messages

**Response**: MCP JSON-RPC responses

#### List Tools

**Endpoint**: `GET /mcp/tools`

**Response**:
```json
{
  "tools": [
    {
      "name": "github_search_repositories",
      "description": "Search GitHub repositories",
      "inputSchema": {
        "type": "object",
        "properties": {
          "query": { "type": "string" },
          "sort": { "type": "string", "enum": ["stars", "forks", "updated"] }
        },
        "required": ["query"]
      }
    }
  ]
}
```

#### List Resources

**Endpoint**: `GET /mcp/resources`

**Response**:
```json
{
  "resources": [
    {
      "uri": "github://user/repo/issues",
      "name": "Repository Issues",
      "description": "GitHub repository issues",
      "mimeType": "application/json"
    }
  ]
}
```

#### Health Check

**Endpoint**: `GET /health`

**Response**:
```json
{
  "service": "proxy",
  "status": "healthy",
  "timestamp": "2025-01-01T12:00:00Z",
  "version": "1.0.0"
}
```

### Adapter Management Service

#### Create Adapter

**Endpoint**: `POST /api/v1/adapters`

**Request**:
```json
{
  "mcpServerId": "github",
  "name": "my-github-adapter",
  "description": "Personal GitHub integration",
  "environmentVariables": {
    "GITHUB_PAT": "ghp_xxx..."
  },
  "authentication": {
    "type": "oauth",
    "oauth": {
      "clientId": "xxx",
      "clientSecret": "xxx"
    }
  }
}
```

**Response**:
```json
{
  "id": "my-github-adapter",
  "mcpServerId": "github",
  "mcpClientConfig": {
    "mcpServers": [
      {
        "url": "http://localhost:8911/api/v1/adapters/my-github-adapter/mcp",
        "auth": {
          "type": "bearer",
          "token": "adapter-session-token"
        }
      }
    ]
  },
  "capabilities": {
    "tools": [
      {
        "name": "github_search_repositories",
        "description": "Search GitHub repositories",
        "inputSchema": { "type": "object", "properties": { "query": { "type": "string" } } }
      }
    ],
    "resources": [],
    "prompts": [],
    "lastRefreshed": "2025-01-01T12:00:00Z"
  },
  "status": "ready",
  "createdAt": "2025-01-01T12:00:00Z"
}
```

**Vue.js Example**:
```javascript
// composables/useAdapters.js
export function useAdapters() {
  const { apiClient } = useApi()
  const adapters = ref([])
  const loading = ref(false)

  const createAdapter = async (adapterData) => {
    loading.value = true
    try {
      const response = await apiClient.post('/api/v1/adapters', adapterData)
      adapters.value.push(response.data)
      return response.data
    } finally {
      loading.value = false
    }
  }

  const listAdapters = async () => {
    const response = await apiClient.get('/api/v1/adapters')
    adapters.value = response.data
    return response.data
  }

  const updateAdapter = async (id, updates) => {
    const response = await apiClient.put(`/api/v1/adapters/${id}`, updates)
    const index = adapters.value.findIndex(a => a.id === id)
    if (index >= 0) {
      adapters.value[index] = response.data
    }
    return response.data
  }

  const deleteAdapter = async (id) => {
    await apiClient.delete(`/api/v1/adapters/${id}`)
    adapters.value = adapters.value.filter(a => a.id !== id)
  }

  const syncAdapterCapabilities = async (id) => {
    const response = await apiClient.post(`/api/v1/adapters/${id}/sync`)
    return response.data
  }

  return {
    adapters: readonly(adapters),
    loading: readonly(loading),
    createAdapter,
    listAdapters,
    updateAdapter,
    deleteAdapter,
    syncAdapterCapabilities
  }
}
```

```vue
<!-- components/adapters/AdapterCreator.vue -->
<template>
  <div class="adapter-creator">
    <div class="header">
      <h2>Create Adapter</h2>
      <p v-if="selectedServer">Creating adapter for <strong>{{ selectedServer.name }}</strong></p>
    </div>

    <div v-if="!selectedServer" class="server-selection">
      <p>Please select an MCP server from the registry first.</p>
      <router-link to="/registry" class="btn-secondary">
        Browse Registry
      </router-link>
    </div>

    <form v-else @submit.prevent="submitForm" class="adapter-form">
      <div class="form-section">
        <h3>Basic Information</h3>

        <div class="form-group">
          <label for="name">Adapter Name *</label>
          <input
            id="name"
            v-model="formData.name"
            type="text"
            required
            placeholder="my-github-adapter"
          />
          <small>Unique name for this adapter</small>
        </div>

        <div class="form-group">
          <label for="description">Description</label>
          <textarea
            id="description"
            v-model="formData.description"
            placeholder="Optional description for this adapter"
            rows="3"
          />
        </div>
      </div>

      <!-- Environment Variables Section -->
      <div v-if="requiredEnvVars.length > 0" class="form-section">
        <h3>Configuration</h3>
        <div class="auth-notice" v-if="selectedServer._meta?.userAuthRequired">
          <div class="notice-icon">🔐</div>
          <div class="notice-content">
            <strong>Authentication Required</strong>
            <p>This server requires authentication. Please provide the necessary credentials below.</p>
          </div>
        </div>

        <div
          v-for="envVar in requiredEnvVars"
          :key="envVar.name"
          class="form-group"
        >
          <label :for="envVar.name">{{ envVar.label }} *</label>
          <input
            :id="envVar.name"
            v-model="formData.environmentVariables[envVar.name]"
            :type="envVar.type || 'text'"
            :placeholder="envVar.placeholder"
            required
          />
          <small>{{ envVar.description }}</small>
        </div>
      </div>

      <!-- Authentication Section -->
      <div v-if="selectedServer._meta?.userAuthRequired" class="form-section">
        <h3>Authentication</h3>

        <div class="auth-method-selector">
          <label>
            <input
              v-model="authMethod"
              type="radio"
              value="oauth"
            />
            OAuth 2.0 (Recommended)
          </label>
          <label>
            <input
              v-model="authMethod"
              type="radio"
              value="apikey"
            />
            API Key
          </label>
        </div>

        <div v-if="authMethod === 'oauth'" class="oauth-config">
          <div class="form-group">
            <label for="clientId">Client ID *</label>
            <input
              id="clientId"
              v-model="formData.authentication.oauth.clientId"
              type="text"
              required
            />
          </div>
          <div class="form-group">
            <label for="clientSecret">Client Secret *</label>
            <input
              id="clientSecret"
              v-model="formData.authentication.oauth.clientSecret"
              type="password"
              required
            />
          </div>
        </div>

        <div v-if="authMethod === 'apikey'" class="apikey-config">
          <div class="form-group">
            <label for="apiKey">API Key *</label>
            <input
              id="apiKey"
              v-model="formData.authentication.apiKey.key"
              type="password"
              required
            />
          </div>
          <div class="form-group">
            <label for="keyLocation">Key Location</label>
            <select v-model="formData.authentication.apiKey.location">
              <option value="header">HTTP Header</option>
              <option value="query">Query Parameter</option>
            </select>
          </div>
          <div class="form-group">
            <label for="keyName">Header/Query Name</label>
            <input
              id="keyName"
              v-model="formData.authentication.apiKey.name"
              type="text"
              placeholder="X-API-Key"
            />
          </div>
        </div>
      </div>

      <div class="form-actions">
        <button type="button" @click="$router.go(-1)" class="btn-secondary">
          Cancel
        </button>
        <button type="submit" :disabled="submitting" class="btn-primary">
          {{ submitting ? 'Creating Adapter...' : 'Create Adapter' }}
        </button>
      </div>
    </form>

    <!-- Success Modal -->
    <div v-if="createdAdapter" class="success-modal" @click.self="closeModal">
      <div class="modal-content">
        <div class="modal-header">
          <h3>✅ Adapter Created Successfully!</h3>
          <button @click="closeModal" class="close-btn">&times;</button>
        </div>

        <div class="modal-body">
          <div class="adapter-info">
            <h4>Adapter Details</h4>
            <dl>
              <dt>Name:</dt>
              <dd>{{ createdAdapter.name }}</dd>
              <dt>Server:</dt>
              <dd>{{ selectedServer.name }}</dd>
              <dt>Status:</dt>
              <dd>{{ createdAdapter.status }}</dd>
            </dl>
          </div>

          <div class="mcp-config">
            <h4>MCP Client Configuration</h4>
            <p>Copy this configuration to your MCP client:</p>
            <div class="config-box">
              <pre>{{ clientConfigJson }}</pre>
              <button @click="copyToClipboard" class="copy-btn">
                📋 Copy
              </button>
            </div>
          </div>

          <div class="capabilities">
            <h4>Discovered Capabilities</h4>
            <div v-if="createdAdapter.capabilities.tools.length > 0" class="capability-section">
              <h5>Tools ({{ createdAdapter.capabilities.tools.length }})</h5>
              <ul>
                <li v-for="tool in createdAdapter.capabilities.tools" :key="tool.name">
                  <strong>{{ tool.name }}</strong>: {{ tool.description }}
                </li>
              </ul>
            </div>
            <div v-if="createdAdapter.capabilities.resources.length > 0" class="capability-section">
              <h5>Resources ({{ createdAdapter.capabilities.resources.length }})</h5>
              <ul>
                <li v-for="resource in createdAdapter.capabilities.resources" :key="resource.uri">
                  <strong>{{ resource.name }}</strong>: {{ resource.description }}
                </li>
              </ul>
            </div>
          </div>
        </div>

        <div class="modal-footer">
          <button @click="closeModal" class="btn-secondary">Close</button>
          <router-link :to="`/adapters/${createdAdapter.id}`" class="btn-primary">
            View Adapter
          </router-link>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAdapters } from '@/composables/useAdapters'
import { useRegistry } from '@/composables/useRegistry'

const route = useRoute()
const router = useRouter()

const { createAdapter } = useAdapters()
const { browseServers } = useRegistry()

const selectedServer = ref(null)
const authMethod = ref('oauth')
const submitting = ref(false)
const createdAdapter = ref(null)

const formData = ref({
  mcpServerId: '',
  name: '',
  description: '',
  environmentVariables: {},
  authentication: {
    type: 'oauth',
    oauth: {
      clientId: '',
      clientSecret: ''
    },
    apiKey: {
      key: '',
      location: 'header',
      name: 'X-API-Key'
    }
  }
})

const requiredEnvVars = computed(() => {
  if (!selectedServer.value) return []

  // Define required environment variables based on server
  const envVars = []

  if (selectedServer.value.id === 'github') {
    envVars.push({
      name: 'GITHUB_PAT',
      label: 'GitHub Personal Access Token',
      type: 'password',
      placeholder: 'ghp_xxxxxxxxxxxxxxxxxxxx',
      description: 'GitHub PAT with repo and user permissions'
    })
  } else if (selectedServer.value.id === 'notion') {
    envVars.push({
      name: 'NOTION_TOKEN',
      label: 'Notion Integration Token',
      type: 'password',
      placeholder: 'secret_xxxxxxxxxxxxxxxxxxxxxxxxxxxx',
      description: 'Notion integration token from your integrations page'
    })
  } else if (selectedServer.value.id === 'sentry') {
    envVars.push({
      name: 'SENTRY_AUTH_TOKEN',
      label: 'Sentry Auth Token',
      type: 'password',
      placeholder: 'your-sentry-auth-token',
      description: 'Sentry auth token with project permissions'
    })
  }

  return envVars
})

const clientConfigJson = computed(() => {
  if (!createdAdapter.value) return ''
  return JSON.stringify(createdAdapter.value.mcpClientConfig, null, 2)
})

const loadServer = async () => {
  const serverId = route.query.server
  if (serverId) {
    try {
      const servers = await browseServers()
      selectedServer.value = servers.find(s => s.id === serverId)
      if (selectedServer.value) {
        formData.value.mcpServerId = serverId
        // Set default name
        formData.value.name = `my-${serverId}-adapter`
      }
    } catch (err) {
      console.error('Failed to load server:', err)
    }
  }
}

const submitForm = async () => {
  try {
    submitting.value = true

    // Set authentication type
    formData.value.authentication.type = authMethod.value

    const adapter = await createAdapter(formData.value)
    createdAdapter.value = adapter

  } catch (err) {
    console.error('Failed to create adapter:', err)
    // Handle error (show toast, etc.)
  } finally {
    submitting.value = false
  }
}

const closeModal = () => {
  createdAdapter.value = null
  router.push('/adapters')
}

const copyToClipboard = async () => {
  try {
    await navigator.clipboard.writeText(clientConfigJson.value)
    // Show success message
  } catch (err) {
    console.error('Failed to copy:', err)
  }
}

onMounted(loadServer)
</script>

<style scoped>
.adapter-creator {
  max-width: 800px;
  margin: 0 auto;
}

.form-section {
  margin-bottom: 2rem;
  padding: 1.5rem;
  border: 1px solid #e1e5e9;
  border-radius: 8px;
  background: #fafbfc;
}

.form-group {
  margin-bottom: 1rem;
}

.form-group label {
  display: block;
  margin-bottom: 0.5rem;
  font-weight: 500;
}

.form-group input,
.form-group textarea,
.form-group select {
  width: 100%;
  padding: 0.5rem;
  border: 1px solid #d1d5db;
  border-radius: 4px;
  font-size: 1rem;
}

.form-group small {
  display: block;
  margin-top: 0.25rem;
  color: #6b7280;
}

.auth-notice {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
  padding: 1rem;
  background: #fff3cd;
  border: 1px solid #ffeaa7;
  border-radius: 6px;
  margin-bottom: 1.5rem;
}

.success-modal {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal-content {
  background: white;
  border-radius: 8px;
  max-width: 600px;
  width: 90%;
  max-height: 80vh;
  overflow-y: auto;
}

.config-box {
  position: relative;
}

.config-box pre {
  background: #f6f8fa;
  padding: 1rem;
  border-radius: 4px;
  font-size: 0.875rem;
  overflow-x: auto;
}

.copy-btn {
  position: absolute;
  top: 0.5rem;
  right: 0.5rem;
  background: #0366d6;
  color: white;
  border: none;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  cursor: pointer;
}
</style>
```

#### List Adapters

**Endpoint**: `GET /api/v1/adapters`

**Response**: Array of adapter objects

#### Update Adapter

**Endpoint**: `PUT /api/v1/adapters/{id}`

**Request**: Partial adapter update

#### Delete Adapter

**Endpoint**: `DELETE /api/v1/adapters/{id}`

#### Sync Capabilities

**Endpoint**: `POST /api/v1/adapters/{id}/sync`

## Data Models

### MCPServer
```typescript
interface MCPServer {
  id: string
  name: string
  description: string
  packages: Package[]
  validationStatus: string
  discoveredAt: string
  url?: string
  _meta: {
    source: string
    userAuthRequired: boolean
    authType: string
    category: string
    documentation?: string
    tags: string[]
  }
}
```

### AdapterResource
```typescript
interface AdapterResource {
  id: string
  name: string
  description: string
  mcpServerId: string
  connectionType: string
  protocol: string
  remoteUrl: string
  environmentVariables: Record<string, string>
  authentication?: AdapterAuthConfig
  mcpFunctionality: MCPFunctionality
  status: string
  createdAt: string
  createdBy: string
}
```

### MCPFunctionality
```typescript
interface MCPFunctionality {
  serverInfo: MCPServerInfo
  tools: MCPTool[]
  resources: MCPResource[]
  prompts: MCPPrompt[]
  lastRefreshed: string
}
```

## Error Handling

### HTTP Status Codes
- `200`: Success
- `201`: Created
- `400`: Bad Request (validation error)
- `401`: Unauthorized (invalid API key)
- `403`: Forbidden (insufficient permissions)
- `404`: Not Found
- `409`: Conflict (duplicate resource)
- `500`: Internal Server Error

### Error Response Format
```json
{
  "error": "Human-readable error message",
  "code": "ERROR_CODE", // Optional
  "details": {} // Optional additional context
}
```

### Vue.js Error Handling
```javascript
// composables/useErrorHandler.js
export function useErrorHandler() {
  const errors = ref([])

  const handleApiError = (error) => {
    let message = 'An unexpected error occurred'

    if (error.response) {
      const status = error.response.status
      const data = error.response.data

      switch (status) {
        case 400:
          message = data.error || 'Invalid request'
          break
        case 401:
          message = 'Authentication required'
          // Redirect to login
          break
        case 403:
          message = 'Access denied'
          break
        case 404:
          message = 'Resource not found'
          break
        case 409:
          message = 'Resource already exists'
          break
        default:
          message = data.error || `Server error (${status})`
      }
    } else if (error.request) {
      message = 'Network error - please check your connection'
    }

    errors.value.push({
      message,
      timestamp: new Date(),
      details: error
    })

    // Show toast notification
    // showToast(message, 'error')
  }

  const clearErrors = () => {
    errors.value = []
  }

  return {
    errors: readonly(errors),
    handleApiError,
    clearErrors
  }
}
```

## Rancher Extension Integration

### Extension Structure
```javascript
// pkg/rancher-extension/index.js
import { RegistryPage } from './pages/RegistryPage.vue'
import { DiscoveryPage } from './pages/DiscoveryPage.vue'
import { AdaptersPage } from './pages/AdaptersPage.vue'

export default {
  name: 'suse-ai-up',
  title: 'SUSE AI Universal Proxy',
  description: 'MCP server registry, discovery, and proxy management',

  routes: [
    {
      name: 'c-cluster-suse-ai-up-registry',
      path: '/c/:cluster/suse-ai-up/registry',
      component: RegistryPage
    },
    {
      name: 'c-cluster-suse-ai-up-discovery',
      path: '/c/:cluster/suse-ai-up/discovery',
      component: DiscoveryPage
    },
    {
      name: 'c-cluster-suse-ai-up-adapters',
      path: '/c/:cluster/suse-ai-up/adapters',
      component: AdaptersPage
    }
  ],

  // Add to Rancher navigation
  navItems: [
    {
      label: 'SUSE AI UP',
      route: 'c-cluster-suse-ai-up-registry',
      icon: 'icon-server'
    }
  ]
}
```

### Rancher Authentication
```javascript
// composables/useRancherAuth.js
import { useAuth } from '@rancher/core'

export function useRancherAuth() {
  const { getToken, getUserId } = useAuth()

  const getAuthHeaders = () => ({
    'Authorization': `Bearer ${getToken()}`,
    'X-API-Key': 'rancher-managed-key',
    'X-User-ID': getUserId()
  })

  return { getAuthHeaders }
}
```

### Rancher UI Components
```vue
<!-- Use Rancher design system -->
<template>
  <div>
    <Banner
      v-if="error"
      color="error"
      :label="error"
    />

    <Card title="MCP Servers">
      <template #body>
        <ResourceTable
          :rows="servers"
          :headers="headers"
          :loading="loading"
        >
          <template #cell:name="{ row }">
            <router-link :to="`/registry/${row.id}`">
              {{ row.name }}
            </router-link>
          </template>
        </ResourceTable>
      </template>
    </Card>
  </div>
</template>

<script>
import { Banner, Card, ResourceTable } from '@rancher/components'
</script>
```

## Best Practices

### State Management
```javascript
// stores/adapters.js
import { defineStore } from 'pinia'
import { useAdapters } from '@/composables/useAdapters'

export const useAdaptersStore = defineStore('adapters', () => {
  const { adapters, loading, createAdapter, listAdapters } = useAdapters()

  const fetchAdapters = async () => {
    await listAdapters()
  }

  const addAdapter = async (adapterData) => {
    const newAdapter = await createAdapter(adapterData)
    // Update local state optimistically
    return newAdapter
  }

  return {
    adapters,
    loading,
    fetchAdapters,
    addAdapter
  }
})
```

### Real-time Updates
```javascript
// composables/usePolling.js
export function usePolling(fetchFn, interval = 5000) {
  const data = ref([])
  const loading = ref(false)

  let intervalId = null

  const startPolling = () => {
    const poll = async () => {
      try {
        loading.value = true
        data.value = await fetchFn()
      } finally {
        loading.value = false
      }
    }

    poll() // Initial fetch
    intervalId = setInterval(poll, interval)
  }

  const stopPolling = () => {
    if (intervalId) {
      clearInterval(intervalId)
    }
  }

  onMounted(startPolling)
  onUnmounted(stopPolling)

  return {
    data: readonly(data),
    loading: readonly(loading),
    startPolling,
    stopPolling
  }
}
```

### Performance Optimization
- Use `keep-alive` for route components
- Implement virtual scrolling for large lists
- Cache API responses where appropriate
- Use lazy loading for heavy components

This comprehensive documentation provides everything needed to build a full-featured Vue.js Rancher extension for the SUSE AI Universal Proxy system! 🚀</content>
<parameter name="filePath">API_DOCUMENTATION.md