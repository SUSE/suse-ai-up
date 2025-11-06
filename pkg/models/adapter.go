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
)

// ServerProtocol represents the protocol used by the adapter
type ServerProtocol string

const (
	ServerProtocolMCP ServerProtocol = "MCP"
)

// AdapterData represents the data for creating or updating an adapter
type AdapterData struct {
	Name                 string            `json:"name" example:"my-adapter"`
	ImageName            string            `json:"imageName,omitempty" example:"nginx"`
	ImageVersion         string            `json:"imageVersion,omitempty" example:"latest"`
	Protocol             ServerProtocol    `json:"protocol" example:"MCP"`
	ConnectionType       ConnectionType    `json:"connectionType" example:"StreamableHttp"`
	EnvironmentVariables map[string]string `json:"environmentVariables"`
	ReplicaCount         int               `json:"replicaCount,omitempty" example:"1"`
	Description          string            `json:"description" example:"My MCP adapter"`
	UseWorkloadIdentity  bool              `json:"useWorkloadIdentity,omitempty" example:"false"`
	// For remote HTTP
	RemoteUrl string `json:"remoteUrl,omitempty" example:"https://remote-mcp.example.com"`
	// For local stdio
	Command string   `json:"command,omitempty" example:"python"`
	Args    []string `json:"args,omitempty" example:"my_server.py"`
	// For MCP client configuration (alternative to Command/Args)
	MCPClientConfig MCPClientConfig `json:"mcpClientConfig,omitempty"`
	// Authentication configuration
	Authentication *AdapterAuthConfig `json:"authentication,omitempty"`
	// MCP Functionality (discovered from server)
	MCPFunctionality *MCPFunctionality `json:"mcpFunctionality,omitempty"`
}

// NewAdapterData creates a new AdapterData with defaults
func NewAdapterData(name, imageName, imageVersion string) *AdapterData {
	return &AdapterData{
		Name:                 name,
		ImageName:            imageName,
		ImageVersion:         imageVersion,
		Protocol:             ServerProtocolMCP,
		ConnectionType:       ConnectionTypeStreamableHttp,
		EnvironmentVariables: make(map[string]string),
		ReplicaCount:         1,
		Description:          "",
		UseWorkloadIdentity:  false,
		RemoteUrl:            "",
		Command:              "",
		Args:                 []string{},
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

// MCPConfigTemplate represents a deployment configuration template for MCP servers
type MCPConfigTemplate struct {
	Command   string            `json:"command"`         // docker, node, python, etc.
	Args      []string          `json:"args,omitempty"`  // command arguments
	Env       map[string]string `json:"env,omitempty"`   // environment variables
	Transport string            `json:"transport"`       // stdio, http, sse, websocket
	Image     string            `json:"image,omitempty"` // Docker image if applicable
}

// MCPServer represents an MCP server entry (enhanced to match MCP registry schema)
type MCPServer struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	Description      string                 `json:"description"`
	Repository       *Repository            `json:"repository,omitempty"`
	Version          string                 `json:"version,omitempty"`
	Packages         []Package              `json:"packages,omitempty"`
	ValidationStatus string                 `json:"validation_status"` // new, approved, certified
	DiscoveredAt     time.Time              `json:"discovered_at"`
	Tools            []MCPTool              `json:"tools,omitempty"`
	Meta             map[string]interface{} `json:"_meta,omitempty"`           // Registry metadata
	ConfigTemplate   *MCPConfigTemplate     `json:"config_template,omitempty"` // Docker/K8s deployment config

	// Legacy fields for backward compatibility
	URL      string `json:"url,omitempty"`
	Protocol string `json:"protocol,omitempty"`
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
}

// MCPToolsConfig represents the mcp_tools configuration for an agent
type MCPToolsConfig []MCPServerConfig

// MCPServerConfig represents the configuration for an MCP server
type MCPServerConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
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
	// Legacy field for backward compatibility
	Token string `json:"token,omitempty"` // For bearer token validation
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
