package config

import (
	"os"
	"strconv"
	"time"
)

// ServiceConfig represents the configuration for a plugin service
type ServiceConfig struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
	Timeout string `json:"timeout"`
}

// PluginServicesConfig holds configuration for all plugin services
type PluginServicesConfig struct {
	SmartAgents ServiceConfig `json:"smartagents"`
	Registry    ServiceConfig `json:"registry"`
}

// Config holds the main application configuration
type Config struct {
	Host   string `json:"host"`
	Port   string `json:"port"`
	APIKey string `json:"api_key"`

	// Plugin services configuration
	Services PluginServicesConfig `json:"services"`

	// Registry configuration
	RegistryEnableOfficial bool     `json:"registry_enable_official"`
	RegistrySyncInterval   string   `json:"registry_sync_interval"`
	RegistryCustomSources  []string `json:"registry_custom_sources"`

	// Authentication
	AuthMode string `json:"auth_mode"`
}

// LoadConfig loads configuration from environment variables
func LoadConfig() *Config {
	cfg := &Config{
		Host:   getEnv("HOST", "localhost"),
		Port:   getEnv("PORT", "8911"),
		APIKey: getEnv("API_KEY", ""),

		Services: PluginServicesConfig{
			SmartAgents: ServiceConfig{
				Enabled: getEnvBool("SMARTAGENTS_ENABLED", true),
				URL:     getEnv("SMARTAGENTS_URL", "http://localhost:8910"),
				Timeout: getEnv("SMARTAGENTS_TIMEOUT", "30s"),
			},
			Registry: ServiceConfig{
				Enabled: getEnvBool("REGISTRY_ENABLED", true),
				URL:     getEnv("REGISTRY_URL", "http://localhost:8912"),
				Timeout: getEnv("REGISTRY_TIMEOUT", "30s"),
			},
		},

		RegistryEnableOfficial: getEnvBool("REGISTRY_ENABLE_OFFICIAL", true),
		RegistrySyncInterval:   getEnv("REGISTRY_SYNC_INTERVAL", "1h"),
		RegistryCustomSources:  []string{}, // TODO: Parse from env

		AuthMode: getEnv("AUTH_MODE", "development"),
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

// GetServiceTimeout returns the timeout duration for a service
func (c *Config) GetServiceTimeout(serviceType string) time.Duration {
	var timeoutStr string
	switch serviceType {
	case "smartagents":
		timeoutStr = c.Services.SmartAgents.Timeout
	case "registry":
		timeoutStr = c.Services.Registry.Timeout

	default:
		timeoutStr = "30s"
	}

	if duration, err := time.ParseDuration(timeoutStr); err == nil {
		return duration
	}
	return 30 * time.Second
}
