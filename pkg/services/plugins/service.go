package plugins

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/plugins"
)

// Service represents the plugins service
type Service struct {
	config  *Config
	server  *http.Server
	manager plugins.PluginServiceManager
	mu      sync.RWMutex
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
	registryManager := &RegistryManager{
		store: clients.NewInMemoryMCPServerStore(),
	}

	service := &Service{
		config:  config,
		manager: plugins.NewServiceManager(nil, registryManager), // TODO: Add config
	}

	return service
}

// Start starts the plugins service
func (s *Service) Start() error {
	log.Printf("Starting MCP Plugins service on port %d", s.config.Port)

	// Setup HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/v1/plugins/register", s.handleRegisterPlugin)
	mux.HandleFunc("/api/v1/plugins/", s.handlePluginByID)
	mux.HandleFunc("/api/v1/plugins", s.handleListPlugins)
	mux.HandleFunc("/api/v1/health/", s.handlePluginHealth)

	// Start health checks
	ctx, cancel := context.WithCancel(context.Background())
	if serviceManager, ok := s.manager.(*plugins.ServiceManager); ok {
		go serviceManager.StartHealthChecks(ctx, s.config.HealthInterval)
	}

	// Store cancel function for cleanup
	s.mu.Lock()
	// TODO: Store cancel function for proper shutdown
	_ = cancel // Prevent unused variable error
	s.mu.Unlock()

	// Start HTTP server
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.Port),
		Handler: mux,
	}

	go func() {
		log.Printf("MCP Plugins HTTP server listening on port %d", s.config.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Start HTTPS server if TLS is configured
	if s.config.TLSPort > 0 {
		tlsConfig := &tls.Config{
			ServerName: "localhost",
		}

		// Try to load certificates
		if s.config.CertFile != "" && s.config.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(s.config.CertFile, s.config.KeyFile)
			if err != nil {
				log.Printf("Failed to load TLS certificates: %v", err)
				if !s.config.AutoTLS {
					return fmt.Errorf("TLS certificates required but failed to load: %w", err)
				}
			} else {
				tlsConfig.Certificates = []tls.Certificate{cert}
			}
		}

		// Generate self-signed certificate if AutoTLS is enabled and no certs loaded
		if s.config.AutoTLS && len(tlsConfig.Certificates) == 0 {
			cert, err := s.generateSelfSignedCert()
			if err != nil {
				return fmt.Errorf("failed to generate self-signed certificate: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{*cert}
			log.Printf("Generated self-signed TLS certificate for plugins service")
		}

		if len(tlsConfig.Certificates) > 0 {
			httpsServer := &http.Server{
				Addr:      fmt.Sprintf(":%d", s.config.TLSPort),
				Handler:   mux,
				TLSConfig: tlsConfig,
			}

			go func() {
				log.Printf("MCP Plugins HTTPS server listening on port %d", s.config.TLSPort)
				if err := httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
					log.Printf("HTTPS server error: %v", err)
				}
			}()
		}
	}

	log.Printf("MCP Plugins service started successfully")
	// Keep the service running
	select {}
}

// Stop stops the plugins service
func (s *Service) Stop() error {
	log.Println("Stopping MCP Plugins service")

	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return s.server.Shutdown(ctx)
	}
	return nil
}

// generateSelfSignedCert generates a self-signed certificate for development
func (s *Service) generateSelfSignedCert() (*tls.Certificate, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"SUSE AI Universal Proxy"},
			CommonName:   "localhost",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost", "127.0.0.1"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, err
	}

	cert := &tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  privateKey,
	}

	return cert, nil
}

// handleHealth handles health check requests
func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"service":   "plugins",
		"timestamp": time.Now(),
	})
}

// handleRegisterPlugin handles plugin registration requests
func (s *Service) handleRegisterPlugin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var registration plugins.ServiceRegistration
	if err := json.NewDecoder(r.Body).Decode(&registration); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := s.manager.RegisterService(&registration); err != nil {
		http.Error(w, fmt.Sprintf("Failed to register plugin: %v", err), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(registration)
}

// handlePluginByID handles requests for specific plugins
func (s *Service) handlePluginByID(w http.ResponseWriter, r *http.Request) {
	pluginID := strings.TrimPrefix(r.URL.Path, "/api/v1/plugins/")
	if pluginID == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetPlugin(w, r, pluginID)
	case http.MethodDelete:
		s.handleDeletePlugin(w, r, pluginID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetPlugin gets a plugin by ID
func (s *Service) handleGetPlugin(w http.ResponseWriter, r *http.Request, pluginID string) {
	registration, exists := s.manager.GetService(pluginID)
	if !exists {
		http.Error(w, "Plugin not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(registration)
}

// handleDeletePlugin deletes a plugin
func (s *Service) handleDeletePlugin(w http.ResponseWriter, r *http.Request, pluginID string) {
	if err := s.manager.UnregisterService(pluginID); err != nil {
		http.Error(w, "Plugin not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleListPlugins lists all registered plugins
func (s *Service) handleListPlugins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	serviceType := query.Get("type")

	var pluginServices []*plugins.ServiceRegistration

	if serviceType != "" {
		var st plugins.ServiceType
		switch serviceType {
		case "smartagents":
			st = plugins.ServiceTypeSmartAgents
		case "registry":
			st = plugins.ServiceTypeRegistry
		case "virtualmcp":
			st = plugins.ServiceTypeVirtualMCP
		default:
			http.Error(w, "Invalid service type", http.StatusBadRequest)
			return
		}
		pluginServices = s.manager.GetServicesByType(st)
	} else {
		pluginServices = s.manager.GetAllServices()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"plugins":    pluginServices,
		"totalCount": len(pluginServices),
	})
}

// handlePluginHealth handles plugin health check requests
func (s *Service) handlePluginHealth(w http.ResponseWriter, r *http.Request) {
	pluginID := strings.TrimPrefix(r.URL.Path, "/api/v1/health/")
	if pluginID == "" {
		http.NotFound(w, r)
		return
	}

	health, exists := s.manager.GetServiceHealth(pluginID)
	if !exists {
		http.Error(w, "Plugin not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

// RegistryManager implements the RegistryManagerInterface for MCP server integration
type RegistryManager struct {
	store clients.MCPServerStore
}

// UploadRegistryEntries uploads MCP server entries to the registry
func (rm *RegistryManager) UploadRegistryEntries(entries []*models.MCPServer) error {
	for _, entry := range entries {
		if err := rm.store.CreateMCPServer(entry); err != nil {
			log.Printf("Failed to upload MCP server %s: %v", entry.ID, err)
			// Continue with other entries
		}
	}
	return nil
}
