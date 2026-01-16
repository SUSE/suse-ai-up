package config

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"suse-ai-up/pkg/network"
)

// ServiceConfig represents the configuration for a plugin service
type ServiceConfig struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
	Timeout string `json:"timeout"`
}

// PluginServicesConfig holds configuration for all plugin services
// Uses a map to allow any service type to be configured
type PluginServicesConfig struct {
	Services map[string]ServiceConfig `json:"services"`
}

// LocalDeploymentConfig holds configuration for local MCP server deployment
type LocalDeploymentConfig struct {
	MinPort int `json:"min_port"`
	MaxPort int `json:"max_port"`
}

// Config holds the main application configuration
type Config struct {
	Host           string                   `json:"host"`
	Port           string                   `json:"port"`
	APIKey         string                   `json:"api_key"`
	AvailableHosts []network.NetworkAddress `json:"available_hosts"`
	PrimaryHost    string                   `json:"primary_host"`

	// Plugin services configuration
	Services PluginServicesConfig `json:"services"`

	// Registry configuration
	RegistrySyncInterval  string   `json:"registry_sync_interval"`
	RegistryCustomSources []string `json:"registry_custom_sources"`
	MCPRegistryURL        string   `json:"mcp_registry_url"`
	RegistryTimeout       string   `json:"registry_timeout"`

	// Local deployment configuration
	LocalDeployment LocalDeploymentConfig `json:"local_deployment"`

	// Authentication
	AuthMode            string `json:"auth_mode"`
	DevMode             bool   `json:"dev_mode"`
	AdminPassword       string `json:"admin_password"`
	ForcePasswordChange bool   `json:"force_password_change"`
	PasswordMinLength   int    `json:"password_min_length"`

	// GitHub OAuth
	GitHubClientID     string   `json:"github_client_id"`
	GitHubClientSecret string   `json:"github_client_secret"`
	GitHubRedirectURI  string   `json:"github_redirect_uri"`
	GitHubAllowedOrgs  []string `json:"github_allowed_orgs"`
	GitHubAdminTeams   []string `json:"github_admin_teams"`

	// Rancher OIDC
	RancherIssuerURL     string   `json:"rancher_issuer_url"`
	RancherClientID      string   `json:"rancher_client_id"`
	RancherClientSecret  string   `json:"rancher_client_secret"`
	RancherRedirectURI   string   `json:"rancher_redirect_uri"`
	RancherAdminGroups   []string `json:"rancher_admin_groups"`
	RancherFallbackLocal bool     `json:"rancher_fallback_local"`

	// Initial Users and Groups
	CreateInitialUsers  bool           `json:"create_initial_users"`
	InitialUsers        []InitialUser  `json:"initial_users"`
	CreateInitialGroups bool           `json:"create_initial_groups"`
	InitialGroups       []InitialGroup `json:"initial_groups"`

	// OpenTelemetry configuration
	OtelEnabled  bool   `json:"otel_enabled"`
	OtelEndpoint string `json:"otel_endpoint"`
	OtelProtocol string `json:"otel_protocol"`
}

// InitialUser represents an initial user to be created
type InitialUser struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Password     string `json:"password,omitempty"`
	Groups       string `json:"groups"` // comma-separated
	AuthProvider string `json:"auth_provider"`
}

// InitialGroup represents an initial group to be created
type InitialGroup struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Permissions string `json:"permissions"` // comma-separated

	// OpenTelemetry configuration
	OtelEnabled  bool   `json:"otel_enabled"`
	OtelEndpoint string `json:"otel_endpoint"`
	OtelProtocol string `json:"otel_protocol"`
}

// LoadConfig loads configuration from environment variables
func LoadConfig() *Config {
	// Detect available network addresses
	availableHosts, err := network.GetAvailableAddresses()
	if err != nil {
		log.Printf("Warning: Failed to detect network addresses: %v", err)
		// Fallback to basic localhost
		availableHosts = []network.NetworkAddress{
			{IP: "127.0.0.1", Interface: "localhost", IsPublic: false, IsLocal: true, Priority: 0},
			{IP: "localhost", Interface: "localhost", IsPublic: false, IsLocal: true, Priority: 1},
		}
	}

	// Get primary host
	primaryHost, err := network.GetPrimaryHost()
	if err != nil {
		log.Printf("Warning: Failed to get primary host: %v", err)
		primaryHost = "localhost"
	}

	// Use HOST env var if set, otherwise use detected primary host
	configuredHost := getEnv("HOST", primaryHost)

	cfg := &Config{
		Host:           configuredHost,
		Port:           getEnv("PORT", "8911"),
		APIKey:         getEnv("API_KEY", ""),
		AvailableHosts: availableHosts,
		PrimaryHost:    primaryHost,

		Services: PluginServicesConfig{
			Services: map[string]ServiceConfig{
				"smartagents": {
					Enabled: getEnvBool("SMARTAGENTS_ENABLED", true),
					URL:     getEnv("SMARTAGENTS_URL", "http://localhost:8910"),
					Timeout: getEnv("SMARTAGENTS_TIMEOUT", "30s"),
				},
				"registry": {
					Enabled: getEnvBool("REGISTRY_ENABLED", true),
					URL:     getEnv("REGISTRY_URL", "http://localhost:8912"),
					Timeout: getEnv("REGISTRY_TIMEOUT", "30s"),
				},
			},
		},

		RegistrySyncInterval:  getEnv("REGISTRY_SYNC_INTERVAL", "1h"),
		RegistryCustomSources: []string{}, // TODO: Parse from env
		MCPRegistryURL:        getEnv("MCP_REGISTRY_URL", ""),
		RegistryTimeout:       getEnv("REGISTRY_TIMEOUT", "30s"),

		LocalDeployment: LocalDeploymentConfig{
			MinPort: getEnvInt("LOCAL_DEPLOYMENT_MIN_PORT", 8000),
			MaxPort: getEnvInt("LOCAL_DEPLOYMENT_MAX_PORT", 19999),
		},

		AuthMode:            getEnv("AUTH_MODE", "development"),
		DevMode:             getEnvBool("DEV_MODE", true),
		AdminPassword:       getEnv("ADMIN_PASSWORD", "admin"),
		ForcePasswordChange: getEnvBool("FORCE_PASSWORD_CHANGE", true),
		PasswordMinLength:   getEnvInt("PASSWORD_MIN_LENGTH", 8),

		// GitHub OAuth
		GitHubClientID:     getEnv("GITHUB_CLIENT_ID", ""),
		GitHubClientSecret: getEnv("GITHUB_CLIENT_SECRET", ""),
		GitHubRedirectURI:  getEnv("GITHUB_REDIRECT_URI", ""),
		GitHubAllowedOrgs:  parseStringSlice(getEnv("GITHUB_ALLOWED_ORGS", "")),
		GitHubAdminTeams:   parseStringSlice(getEnv("GITHUB_ADMIN_TEAMS", "")),

		// Rancher OIDC
		RancherIssuerURL:     getEnv("RANCHER_ISSUER_URL", ""),
		RancherClientID:      getEnv("RANCHER_CLIENT_ID", ""),
		RancherClientSecret:  getEnv("RANCHER_CLIENT_SECRET", ""),
		RancherRedirectURI:   getEnv("RANCHER_REDIRECT_URI", ""),
		RancherAdminGroups:   parseStringSlice(getEnv("RANCHER_ADMIN_GROUPS", "")),
		RancherFallbackLocal: getEnvBool("RANCHER_FALLBACK_LOCAL", true),

		// Initial Users and Groups
		CreateInitialUsers:  getEnvBool("CREATE_INITIAL_USERS", true),
		InitialUsers:        parseInitialUsers(getEnv("INITIAL_USERS", "")),
		CreateInitialGroups: getEnvBool("CREATE_INITIAL_GROUPS", true),
		InitialGroups:       parseInitialGroups(getEnv("INITIAL_GROUPS", "")),

		// OpenTelemetry configuration
		OtelEnabled:  getEnvBool("OTEL_ENABLED", false),
		OtelEndpoint: getEnv("OTEL_ENDPOINT", "http://localhost:4318"),
		OtelProtocol: getEnv("OTEL_PROTOCOL", "grpc"),
	}

	return cfg
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvBool gets a boolean environment variable with a default value
func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// getEnvInt gets an integer environment variable with a default value
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// parseStringSlice parses a comma-separated string into a slice
func parseStringSlice(value string) []string {
	if value == "" {
		return []string{}
	}
	var result []string
	for _, item := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// parseInitialUsers parses initial users from JSON string
func parseInitialUsers(jsonStr string) []InitialUser {
	if jsonStr == "" {
		return defaultInitialUsers()
	}
	var users []InitialUser
	if err := json.Unmarshal([]byte(jsonStr), &users); err != nil {
		log.Printf("Warning: Failed to parse INITIAL_USERS: %v, using defaults", err)
		return defaultInitialUsers()
	}
	return users
}

// parseInitialGroups parses initial groups from JSON string
func parseInitialGroups(jsonStr string) []InitialGroup {
	if jsonStr == "" {
		return defaultInitialGroups()
	}
	var groups []InitialGroup
	if err := json.Unmarshal([]byte(jsonStr), &groups); err != nil {
		log.Printf("Warning: Failed to parse INITIAL_GROUPS: %v, using defaults", err)
		return defaultInitialGroups()
	}
	return groups
}

// defaultInitialUsers returns default initial users
func defaultInitialUsers() []InitialUser {
	return []InitialUser{
		{
			ID:           "admin",
			Name:         "System Administrator",
			Email:        "admin@suse.ai",
			Password:     "", // Will use ADMIN_PASSWORD env var
			Groups:       "mcp-admins",
			AuthProvider: "local",
		},
	}
}

// defaultInitialGroups returns default initial groups
func defaultInitialGroups() []InitialGroup {
	return []InitialGroup{
		{
			ID:          "mcp-admins",
			Name:        "MCP Administrators",
			Description: "Full administrative access to all MCP proxy features",
			Permissions: "user:create,user:read,user:update,user:delete,group:create,group:read,group:update,group:delete,adapter:create,adapter:read,adapter:update,adapter:delete,adapter:assign,server:create,server:read,server:update,server:delete,discovery:create,discovery:read,discovery:update,discovery:delete,registry:create,registry:read,registry:update,registry:delete",
		},
		{
			ID:          "mcp-users",
			Name:        "MCP Users",
			Description: "Basic access to MCP servers with limited adapter operations",
			Permissions: "server:read,adapter:read,adapter:create,adapter:assign",
		},
	}
}

// GetServiceTimeout returns the timeout duration for a service
func (c *Config) GetServiceTimeout(serviceType string) time.Duration {
	if serviceConfig, exists := c.Services.Services[serviceType]; exists {
		if duration, err := time.ParseDuration(serviceConfig.Timeout); err == nil {
			return duration
		}
	}
	return 30 * time.Second
}

// GetServerURLs returns all available server URLs for the current configuration
func (c *Config) GetServerURLs() []string {
	var urls []string

	for _, addr := range c.AvailableHosts {
		url := network.FormatHostURL(addr.IP, c.Port)
		urls = append(urls, "http://"+url)
	}

	return urls
}
