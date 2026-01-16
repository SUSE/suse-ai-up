package plugins

import (
	"log"
	"net/http"
	"sync"
	"time"

	"suse-ai-up/internal/handlers"
	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/plugins"
)

// Service represents the plugins service
type Service struct {
	config     *Config
	server     *http.Server
	manager    plugins.PluginServiceManager
	shutdownCh chan struct{}
	mu         sync.RWMutex
}

// Config holds plugins service configuration
type Config struct {
	Port           int           `json:"port"`
	TLSPort        int           `json:"tls_port"`
	HealthInterval time.Duration `json:"health_interval"`
	AutoTLS        bool          `json:"auto_tls"`
	CertFile       string        `json:"cert_file"`
	KeyFile        string        `json:"key_file"`
}

// NewService creates a new plugins service
func NewService(config *Config) *Service {
	if config.HealthInterval == 0 {
		config.HealthInterval = 30 * time.Second
	}

	// Create registry manager for MCP server integration
	registryManager := handlers.NewDefaultRegistryManager(clients.NewInMemoryMCPServerStore())

	service := &Service{
		config:     config,
		manager:    plugins.NewServiceManager(nil, registryManager), // TODO: Add config
		shutdownCh: make(chan struct{}),
	}

	return service
}

// Start starts the plugins service
// Start starts the MCP Plugins service (unified architecture - no longer starts HTTP servers)
func (s *Service) Start() error {
	log.Printf("Plugins service initialized (routes handled by main Gin server)")
	return nil
}
