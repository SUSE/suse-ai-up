package config

import (
	"log"
	"os"
	"strconv"
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

	// Local deployment configuration
	LocalDeployment LocalDeploymentConfig `json:"local_deployment"`

	// Authentication
	AuthMode string `json:"auth_mode"`

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

		LocalDeployment: LocalDeploymentConfig{
			MinPort: getEnvInt("LOCAL_DEPLOYMENT_MIN_PORT", 8000),
			MaxPort: getEnvInt("LOCAL_DEPLOYMENT_MAX_PORT", 19999),
		},

		AuthMode: getEnv("AUTH_MODE", "development"),

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

// GetSwaggerHost returns the best host to use for swagger documentation
func (c *Config) GetSwaggerHost() string {
	// Use the primary host with port for swagger
	return network.FormatHostURL(c.PrimaryHost, c.Port)
}
