package models

import (
	"time"
)

// ConnectionType represents the connection type for the adapter
type ConnectionType string

const (
	ConnectionTypeSSE            ConnectionType = "SSE"
	ConnectionTypeStreamableHttp ConnectionType = "StreamableHttp"
	ConnectionTypeRemoteHttp     ConnectionType = "RemoteHttp"
	ConnectionTypeLocalStdio     ConnectionType = "LocalStdio"
	ConnectionTypeSidecarStdio   ConnectionType = "SidecarStdio"
)

// ServerProtocol represents the protocol used by the adapter
type ServerProtocol string

const (
	ServerProtocolMCP ServerProtocol = "MCP"
)

// AdapterLifecycleStatus represents the lifecycle status of an adapter
type AdapterLifecycleStatus string

const (
	AdapterLifecycleStatusNotReady AdapterLifecycleStatus = "not ready"
	AdapterLifecycleStatusReady    AdapterLifecycleStatus = "ready"
	AdapterLifecycleStatusError    AdapterLifecycleStatus = "error"
)

// SidecarConfig represents configuration for sidecar container deployment
type SidecarConfig struct {
	// Command execution type
	CommandType string `json:"commandType" example:"docker"` // "docker", "npx", "python", "pip"

	// Command and arguments
	Command string   `json:"command" example:"docker"`
	Args    []string `json:"args,omitempty" example:"run,-i,--rm,-e,VAR=value,image:tag,cmd"`

	// Environment variables
	Env []map[string]string `json:"env,omitempty"`

	// Port assignment
	Port int `json:"port" example:"8000"` // Randomly assigned port

	// Metadata
	Source      string `json:"source" example:"manual-config"`
	LastUpdated string `json:"lastUpdated,omitempty" example:"2024-01-01T00:00:00Z"`
	ProjectURL  string `json:"projectURL,omitempty" example:"https://github.com/user/repo"`
	ReleaseURL  string `json:"releaseURL,omitempty" example:"https://github.com/user/repo/releases/tag/v1.0.0"`
}

// AdapterData represents the data for creating or updating an adapter
type AdapterData struct {
	Name                 string                 `json:"name" example:"my-adapter"`
	ImageName            string                 `json:"imageName,omitempty" example:"nginx"`
	ImageVersion         string                 `json:"imageVersion,omitempty" example:"latest"`
	Protocol             ServerProtocol         `json:"protocol" example:"MCP"`
	ConnectionType       ConnectionType         `json:"connectionType" example:"StreamableHttp"`
	Status               AdapterLifecycleStatus `json:"status" example:"ready"`
	EnvironmentVariables map[string]string      `json:"environmentVariables"`
	ReplicaCount         int                    `json:"replicaCount,omitempty" example:"1"`
	Description          string                 `json:"description" example:"My MCP adapter"`
	UseWorkloadIdentity  bool                   `json:"useWorkloadIdentity,omitempty" example:"false"`
	// Adapter MCP endpoint URL
	URL string `json:"url,omitempty" example:"http://localhost:8911/api/v1/adapters/my-adapter/mcp"`
	// For remote HTTP
	RemoteUrl string `json:"remoteUrl,omitempty" example:"https://remote-mcp.example.com"`
	// For VirtualMCP API configuration
	ApiBaseUrl string `json:"apiBaseUrl,omitempty" example:"http://localhost:8000"`
	// For VirtualMCP tools configuration
	Tools []interface{} `json:"tools,omitempty"`
	// For local stdio
	Command string   `json:"command,omitempty" example:"python"`
	Args    []string `json:"args,omitempty" example:"my_server.py"`
	// For MCP client configuration (alternative to Command/Args)
	MCPClientConfig MCPClientConfig `json:"mcpClientConfig,omitempty"`
	// Authentication configuration
	Authentication *AdapterAuthConfig `json:"authentication,omitempty"`
	// MCP Functionality (discovered from server)
	MCPFunctionality *MCPFunctionality `json:"mcpFunctionality,omitempty"`
	// For sidecar stdio deployment
	SidecarConfig *SidecarConfig `json:"sidecarConfig,omitempty"`
}

// NewAdapterData creates a new AdapterData with defaults
func NewAdapterData(name, imageName, imageVersion string) *AdapterData {
	return &AdapterData{
		Name:                 name,
		ImageName:            imageName,
		ImageVersion:         imageVersion,
		Protocol:             ServerProtocolMCP,
		ConnectionType:       ConnectionTypeStreamableHttp,
		Status:               AdapterLifecycleStatusNotReady,
		EnvironmentVariables: make(map[string]string),
		ReplicaCount:         1,
		Description:          "",
		UseWorkloadIdentity:  false,
		RemoteUrl:            "",
		Command:              "",
		Args:                 []string{},
		SidecarConfig:        nil,
	}
}

// AdapterResource represents a full adapter resource with metadata
type AdapterResource struct {
	AdapterData
	ID            string    `json:"id" example:"my-adapter"`
	CreatedBy     string    `json:"createdBy" example:"user@example.com"`
	CreatedAt     time.Time `json:"createdAt"`
	LastUpdatedAt time.Time `json:"lastUpdatedAt"`
}

// Create creates a new AdapterResource from AdapterData
func (ar *AdapterResource) Create(data AdapterData, createdBy string, createdAt time.Time) {
	ar.AdapterData = data
	ar.ID = data.Name
	ar.CreatedBy = createdBy
	ar.CreatedAt = createdAt
	ar.LastUpdatedAt = time.Now().UTC()

	// Merge environment variables from MCPClientConfig into EnvironmentVariables
	if len(data.MCPClientConfig.MCPServers) > 0 {
		if ar.EnvironmentVariables == nil {
			ar.EnvironmentVariables = make(map[string]string)
		}
		for _, serverConfig := range data.MCPClientConfig.MCPServers {
			for k, v := range serverConfig.Env {
				ar.EnvironmentVariables[k] = v
			}
			break // Only use the first server config
		}
	}
}

// AdapterStatus represents the status of a deployed adapter
type AdapterStatus struct {
	ReadyReplicas     *int   `json:"readyReplicas" example:"1"`
	UpdatedReplicas   *int   `json:"updatedReplicas" example:"1"`
	AvailableReplicas *int   `json:"availableReplicas" example:"1"`
	Image             string `json:"image" example:"nginx:latest"`
	ReplicaStatus     string `json:"replicaStatus" example:"Healthy"`
}

// DiscoveredServer represents a found MCP server
type DiscoveredServer struct {
	ID                 string            `json:"id" example:"server-123"`
	Name               string            `json:"name,omitempty" example:"MCP Example Server"`
	Address            string            `json:"address" example:"http://192.168.1.100:8000"`
	Protocol           ServerProtocol    `json:"protocol" example:"MCP"`
	Connection         ConnectionType    `json:"connection" example:"StreamableHttp"`
	Status             string            `json:"status" example:"healthy"`
	LastSeen           time.Time         `json:"lastSeen"`
	Metadata           map[string]string `json:"metadata"`
	VulnerabilityScore string            `json:"vulnerability_score" example:"high"`

	// Enhanced fields for deep interrogation
	Capabilities      *McpCapabilities      `json:"capabilities,omitempty"`
	Tools             []McpTool             `json:"tools,omitempty"`
	Resources         []McpResource         `json:"resources,omitempty"`
	Prompts           []McpPrompt           `json:"prompts,omitempty"`
	ResourceTemplates []McpResourceTemplate `json:"resource_templates,omitempty"`
	AuthInfo          *AuthAnalysis         `json:"auth_info,omitempty"`
	LastDeepScan      time.Time             `json:"last_deep_scan,omitempty"`
	ServerVersion     string                `json:"server_version,omitempty"`
	ProtocolVersion   string                `json:"protocol_version,omitempty"`
}

// ScanConfig represents configuration for network scanning
type ScanConfig struct {
	ScanRanges       []string `json:"scanRanges" example:"192.168.1.0/24,10.0.0.1-10.0.0.10"`
	Ports            []string `json:"ports" example:"8000,8001,9000-9100"`
	Timeout          string   `json:"timeout" example:"30s"`
	MaxConcurrent    int      `json:"maxConcurrent" example:"10"`
	ExcludeProxy     *bool    `json:"excludeProxy,omitempty" example:"true"` // Default: true
	ExcludeAddresses []string `json:"excludeAddresses,omitempty"`            // Additional addresses to skip
}

// ScanJob represents a running or completed scan
type ScanJob struct {
	ID        string             `json:"id" example:"scan-12345"`
	Status    string             `json:"status" example:"running"`
	StartTime time.Time          `json:"startTime"`
	Config    ScanConfig         `json:"config"`
	Results   []DiscoveredServer `json:"results,omitempty"`
	Error     string             `json:"error,omitempty"`
}

// GitHubConfig represents GitHub-specific MCP server configuration
type GitHubConfig struct {
	APIEndpoint string `json:"api_endpoint,omitempty"` // GitHub API endpoint (defaults to https://api.githubcopilot.com/mcp/)
	Token       string `json:"token,omitempty"`        // GitHub Personal Access Token
	Owner       string `json:"owner,omitempty"`        // Repository owner (for repo-specific servers)
	Repo        string `json:"repo,omitempty"`         // Repository name (for repo-specific servers)
}

// MCPServer represents an MCP server entry (enhanced to match MCP registry schema)
type MCPServer struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	Image            string                 `json:"image,omitempty"` // Docker image for containerized servers
	Description      string                 `json:"description"`
	Version          string                 `json:"version"`
	Repository       Repository             `json:"repository,omitempty"`
	Packages         []Package              `json:"packages"`
	Tools            []MCPTool              `json:"tools,omitempty"`
	ValidationStatus string                 `json:"validation_status"`
	DiscoveredAt     time.Time              `json:"discovered_at"`
	URL              string                 `json:"url,omitempty"` // Legacy URL field for remote servers
	Meta             map[string]interface{} `json:"_meta,omitempty"`
	GitHubConfig     *GitHubConfig          `json:"github_config,omitempty"`
	RouteAssignments []RouteAssignment      `json:"routeAssignments,omitempty"` // User/group access control
	AutoSpawn        *AutoSpawnConfig       `json:"autoSpawn,omitempty"`        // Auto-spawning configuration
}

// convertToSerializable converts interface{} values to JSON-serializable types
func convertToSerializable(v interface{}) interface{} {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case map[interface{}]interface{}:
		// Convert map[interface{}]interface{} to map[string]interface{}
		result := make(map[string]interface{})
		for k, v := range val {
			if keyStr, ok := k.(string); ok {
				result[keyStr] = convertToSerializable(v)
			}
		}
		return result
	case []interface{}:
		// Handle slices
		result := make([]interface{}, len(val))
		for i, item := range val {
			result[i] = convertToSerializable(item)
		}
		return result
	case map[string]interface{}:
		// Recursively convert nested maps
		result := make(map[string]interface{})
		for k, v := range val {
			result[k] = convertToSerializable(v)
		}
		return result
	default:
		return v
	}
}

// Repository represents repository information for an MCP server
type Repository struct {
	URL    string `json:"url"`
	Source string `json:"source"` // github, gitlab, etc.
}

// Package represents a server package (stdio, docker, etc.)
type Package struct {
	RegistryType         string                `json:"registryType"` // oci, npm, etc.
	Identifier           string                `json:"identifier"`   // docker.io/user/image:tag
	Transport            Transport             `json:"transport"`
	EnvironmentVariables []EnvironmentVariable `json:"environmentVariables,omitempty"`
}

// Transport defines how to connect to the server
type Transport struct {
	Type string `json:"type"` // stdio, sse, websocket, http
	// Additional transport-specific fields can be added here
}

// EnvironmentVariable represents an environment variable for the server
type EnvironmentVariable struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Format      string `json:"format,omitempty"`   // string, number, boolean
	IsSecret    bool   `json:"isSecret,omitempty"` // true for sensitive values
	Default     string `json:"default,omitempty"`  // default value if any
}

// MCPTool represents an MCP tool
type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
	// VirtualMCP specific fields
	SourceType string                 `json:"source_type,omitempty"` // "api", "database", "graphql"
	Config     map[string]interface{} `json:"config,omitempty"`      // Tool-specific configuration
}

// MCPToolsConfig represents the mcp_tools configuration for an agent
type MCPToolsConfig []MCPServerConfig

// MCPServerConfig represents the configuration for an MCP server
type MCPServerConfig struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// MCPClientConfig represents the full MCP client configuration format
type MCPClientConfig struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
}

// MCPServerInfo represents MCP server information from initialize response
type MCPServerInfo struct {
	Name         string                 `json:"name"`
	Version      string                 `json:"version"`
	Protocol     string                 `json:"protocol"`
	Capabilities map[string]interface{} `json:"capabilities"`
}

// MCPPrompt represents an MCP prompt
type MCPPrompt struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Arguments   []MCPArgument `json:"arguments,omitempty"`
}

// MCPArgument represents an MCP prompt argument
type MCPArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// MCPResource represents an MCP resource
type MCPResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// MCPFunctionality represents discovered MCP server capabilities
type MCPFunctionality struct {
	ServerInfo    MCPServerInfo `json:"serverInfo"`
	Tools         []MCPTool     `json:"tools,omitempty"`
	Prompts       []MCPPrompt   `json:"prompts,omitempty"`
	Resources     []MCPResource `json:"resources,omitempty"`
	LastRefreshed time.Time     `json:"lastRefreshed"`
}

// BearerTokenConfig represents bearer token authentication configuration
type BearerTokenConfig struct {
	Token     string    `json:"token,omitempty"` // Static token
	Dynamic   bool      `json:"dynamic"`         // Use token manager
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
}

// OAuthConfig represents OAuth authentication configuration
type OAuthConfig struct {
	ClientID     string   `json:"clientId,omitempty"`
	ClientSecret string   `json:"clientSecret,omitempty"`
	AuthURL      string   `json:"authUrl,omitempty"`
	TokenURL     string   `json:"tokenUrl,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
	RedirectURI  string   `json:"redirectUri,omitempty"`
}

// BasicAuthConfig represents basic authentication configuration
type BasicAuthConfig struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// APIKeyConfig represents API key authentication configuration
type APIKeyConfig struct {
	Key      string `json:"key,omitempty"`
	Location string `json:"location,omitempty"` // "header", "query", "cookie"
	Name     string `json:"name,omitempty"`     // Header name, query param, or cookie name
}

// AdapterAuthConfig represents authentication configuration for an adapter
type AdapterAuthConfig struct {
	Required    bool               `json:"required"` // true = require auth, false = optional
	Type        string             `json:"type"`     // "bearer", "oauth", "basic", "apikey", "none"
	BearerToken *BearerTokenConfig `json:"bearerToken,omitempty"`
	OAuth       *OAuthConfig       `json:"oauth,omitempty"`
	Basic       *BasicAuthConfig   `json:"basic,omitempty"`
	APIKey      *APIKeyConfig      `json:"apiKey,omitempty"`
}

// UserAuthProvider represents supported user authentication providers
type UserAuthProvider string

const (
	UserAuthProviderLocal   UserAuthProvider = "local"
	UserAuthProviderGitHub  UserAuthProvider = "github"
	UserAuthProviderRancher UserAuthProvider = "rancher"
)

// UserAuthConfig represents the complete authentication configuration
type UserAuthConfig struct {
	Mode    string             `json:"mode"`     // "local", "github", "rancher", "dev"
	DevMode bool               `json:"dev_mode"` // Bypass authentication in dev
	Local   *LocalAuthConfig   `json:"local,omitempty"`
	GitHub  *GitHubAuthConfig  `json:"github,omitempty"`
	Rancher *RancherAuthConfig `json:"rancher,omitempty"`
}

// LocalAuthConfig represents local password authentication configuration
type LocalAuthConfig struct {
	DefaultAdminPassword string `json:"default_admin_password,omitempty"`
	ForcePasswordChange  bool   `json:"force_password_change"`
	PasswordMinLength    int    `json:"password_min_length"`
}

// GitHubAuthConfig represents GitHub OAuth configuration
type GitHubAuthConfig struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RedirectURI  string   `json:"redirect_uri"`
	AllowedOrgs  []string `json:"allowed_orgs,omitempty"`
	AdminTeams   []string `json:"admin_teams,omitempty"`
}

// RancherAuthConfig represents Rancher OIDC configuration
type RancherAuthConfig struct {
	IssuerURL     string   `json:"issuer_url"`
	ClientID      string   `json:"client_id"`
	ClientSecret  string   `json:"client_secret"`
	RedirectURI   string   `json:"redirect_uri"`
	AdminGroups   []string `json:"admin_groups"`
	FallbackLocal bool     `json:"fallback_local"`
}

// AuthToken represents a JWT token for user authentication
type AuthToken struct {
	Token     string    `json:"token"`
	TokenType string    `json:"token_type"` // "Bearer"
	ExpiresAt time.Time `json:"expires_at"`
	UserID    string    `json:"user_id"`
	Provider  string    `json:"provider"`
}

// RegistrySource represents a source of MCP registry data
type RegistrySource struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"` // official, file, http, git
	URL       string    `json:"url"`
	Enabled   bool      `json:"enabled"`
	LastSync  time.Time `json:"lastSync,omitempty"`
	SyncError string    `json:"syncError,omitempty"`
	Priority  int       `json:"priority"` // Higher priority sources are preferred
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// McpCapabilities represents MCP server capabilities
type McpCapabilities struct {
	Tools        bool `json:"tools"`
	Prompts      bool `json:"prompts"`
	Resources    bool `json:"resources"`
	Logging      bool `json:"logging"`
	Completions  bool `json:"completions"`
	Experimental bool `json:"experimental"`
}

// McpTool represents a discovered MCP tool
type McpTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// McpResource represents a discovered MCP resource
type McpResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mime_type,omitempty"`
}

// McpResourceTemplate represents a discovered MCP resource template
type McpResourceTemplate struct {
	URITemplate string `json:"uri_template"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mime_type,omitempty"`
}

// McpPrompt represents a discovered MCP prompt
type McpPrompt struct {
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Arguments   []McpArgument `json:"arguments,omitempty"`
}

// McpArgument represents an MCP prompt argument
type McpArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
}

// AuthAnalysis represents authentication analysis results
type AuthAnalysis struct {
	Required           bool     `json:"required"`
	Type               string   `json:"type"` // "none", "basic", "bearer", "apikey", "oauth", "missing"
	DetectedMechanisms []string `json:"detected_mechanisms"`
	Vulnerabilities    []string `json:"vulnerabilities"`
	Confidence         string   `json:"confidence"` // "high", "medium", "low"
}

// CapabilityValidation represents validation results for MCP capabilities
type CapabilityValidation struct {
	ToolsValid     bool     `json:"tools_valid"`
	ResourcesValid bool     `json:"resources_valid"`
	PromptsValid   bool     `json:"prompts_valid"`
	Issues         []string `json:"issues"`
}

// User represents a system user
type User struct {
	ID                string     `json:"id" example:"user123"`
	Name              string     `json:"name" example:"John Doe"`
	Email             string     `json:"email" example:"john@example.com"`
	Groups            []string   `json:"groups" example:"[\"mcp-users\",\"weather-team\"]"`
	AuthProvider      string     `json:"auth_provider,omitempty" example:"local"`
	ExternalID        string     `json:"external_id,omitempty" example:"github123"`
	ProviderGroups    []string   `json:"provider_groups,omitempty" example:"[\"org/team\"]"`
	PasswordHash      string     `json:"-"` // Never serialize password hash
	PasswordChangedAt *time.Time `json:"password_changed_at,omitempty"`
	LastLoginAt       *time.Time `json:"last_login_at,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

// Group represents a user group with permissions
type Group struct {
	ID          string    `json:"id" example:"mcp-users"`
	Name        string    `json:"name" example:"MCP Users"`
	Description string    `json:"description" example:"Users with access to MCP servers"`
	Members     []string  `json:"members" example:"[\"user123\",\"user456\"]"`
	Permissions []string  `json:"permissions" example:"[\"server:read\",\"adapter:create\"]"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// RouteAssignment represents user/group assignment to server routes
type RouteAssignment struct {
	ID          string    `json:"id" example:"assignment-123"`
	ServerID    string    `json:"serverId" example:"mcp-bugzilla"`
	UserIDs     []string  `json:"userIds,omitempty" example:"[\"user123\"]"`
	GroupIDs    []string  `json:"groupIds,omitempty" example:"[\"mcp-users\"]"`
	AutoSpawn   bool      `json:"autoSpawn" example:"true"`
	Permissions string    `json:"permissions" example:"read"` // "read", "write", "admin"
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// AutoSpawnConfig represents auto-spawning configuration for servers
type AutoSpawnConfig struct {
	Enabled          bool              `json:"enabled"`
	ConnectionType   ConnectionType    `json:"connectionType"`             // "LocalStdio" or "StreamableHttp"
	Command          string            `json:"command,omitempty"`          // for stdio
	Args             []string          `json:"args,omitempty"`             // for stdio
	ImageName        string            `json:"imageName,omitempty"`        // for containers
	ImageVersion     string            `json:"imageVersion,omitempty"`     // for containers
	DefaultEnv       map[string]string `json:"defaultEnv,omitempty"`       // default env vars
	DeploymentMethod string            `json:"deploymentMethod,omitempty"` // "docker", "local"
}
