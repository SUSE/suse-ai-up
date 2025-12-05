package registry

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
	"suse-ai-up/pkg/middleware"
	"suse-ai-up/pkg/models"
)

// Service represents the registry service
type Service struct {
	config      *Config
	server      *http.Server
	store       clients.MCPServerStore
	syncManager *SyncManager
	mu          sync.RWMutex
}

// Config holds registry service configuration
type Config struct {
	Port           int           `json:"port"`
	TLSPort        int           `json:"tls_port"`
	ConfigFile     string        `json:"config_file"`
	EnableOfficial bool          `json:"enable_official"`
	EnableDocker   bool          `json:"enable_docker"`
	SyncInterval   time.Duration `json:"sync_interval"`
	AutoTLS        bool          `json:"auto_tls"`
	CertFile       string        `json:"cert_file"`
	KeyFile        string        `json:"key_file"`
}

// NewService creates a new registry service
func NewService(config *Config) *Service {
	if config.SyncInterval == 0 {
		config.SyncInterval = 24 * time.Hour // Default to daily sync
	}

	service := &Service{
		config: config,
		store:  clients.NewInMemoryMCPServerStore(),
	}

	// Initialize sync manager
	service.syncManager = NewSyncManager(service.store)

	return service
}

// Start starts the registry service
func (s *Service) Start() error {
	log.Printf("Starting MCP Registry service on port %d", s.config.Port)

	// Setup HTTP routes with CORS middleware
	mux := http.NewServeMux()
	mux.HandleFunc("/health", middleware.CORSMiddleware(s.handleHealth))
	mux.HandleFunc("/api/v1/registry/browse", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(s.handleBrowse)))
	mux.HandleFunc("/api/v1/registry/", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(s.handleRegistryByID)))
	mux.HandleFunc("/api/v1/registry/upload", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(s.handleUpload)))
	mux.HandleFunc("/api/v1/registry/upload/bulk", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(s.handleBulkUpload)))
	mux.HandleFunc("/api/v1/registry/sync/official", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(s.handleSyncOfficial)))
	mux.HandleFunc("/api/v1/registry/sync/docker", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(s.handleSyncDocker)))

	// Start sync operations if enabled
	if s.config.EnableOfficial {
		go s.startPeriodicSync("official", s.syncOfficialRegistry)
	}
	if s.config.EnableDocker {
		go s.startPeriodicSync("docker", s.syncDockerRegistry)
	}

	// Start HTTP server
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.Port),
		Handler: mux,
	}

	go func() {
		log.Printf("MCP Registry HTTP server listening on port %d", s.config.Port)
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
			log.Printf("Generated self-signed TLS certificate for registry service")
		}

		if len(tlsConfig.Certificates) > 0 {
			httpsServer := &http.Server{
				Addr:      fmt.Sprintf(":%d", s.config.TLSPort),
				Handler:   mux,
				TLSConfig: tlsConfig,
			}

			go func() {
				log.Printf("MCP Registry HTTPS server listening on port %d", s.config.TLSPort)
				if err := httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
					log.Printf("HTTPS server error: %v", err)
				}
			}()
		}
	}

	log.Printf("MCP Registry service started successfully")
	// Keep the service running
	select {}
}

// Stop stops the registry service
func (s *Service) Stop() error {
	log.Println("Stopping MCP Registry service")
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

// startPeriodicSync starts periodic sync for a registry source
func (s *Service) startPeriodicSync(sourceName string, syncFunc func() error) {
	ticker := time.NewTicker(s.config.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			log.Printf("Starting periodic sync for %s registry", sourceName)
			if err := syncFunc(); err != nil {
				log.Printf("Failed to sync %s registry: %v", sourceName, err)
			} else {
				log.Printf("Successfully synced %s registry", sourceName)
			}
		}
	}
}

// syncOfficialRegistry syncs the official MCP registry
func (s *Service) syncOfficialRegistry() error {
	return s.syncManager.SyncOfficialRegistry(context.Background())
}

// syncDockerRegistry syncs the Docker MCP registry
func (s *Service) syncDockerRegistry() error {
	return s.syncManager.SyncDockerRegistry(context.Background())
}

// handleHealth handles health check requests
func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"service":   "registry",
		"timestamp": time.Now(),
	})
}

// handleBrowse handles registry browsing requests
func (s *Service) handleBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	searchQuery := query.Get("q")
	transportType := query.Get("transport")
	registryType := query.Get("registryType")
	validationStatus := query.Get("validationStatus")

	// Get all servers
	servers := s.store.ListMCPServers()

	// Apply filters
	var filtered []*models.MCPServer
	for _, server := range servers {
		if s.matchesFilters(server, searchQuery, transportType, registryType, validationStatus) {
			filtered = append(filtered, server)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(filtered)
}

// matchesFilters checks if a server matches the given filters
func (s *Service) matchesFilters(server *models.MCPServer, query, transport, registryType, validationStatus string) bool {
	// Search query filter
	if query != "" {
		queryLower := strings.ToLower(query)
		if !strings.Contains(strings.ToLower(server.Name), queryLower) &&
			!strings.Contains(strings.ToLower(server.Description), queryLower) {
			return false
		}
	}

	// Transport type filter
	if transport != "" {
		hasTransport := false
		for _, pkg := range server.Packages {
			if pkg.Transport.Type == transport {
				hasTransport = true
				break
			}
		}
		if !hasTransport {
			return false
		}
	}

	// Registry type filter
	if registryType != "" {
		hasRegistryType := false
		for _, pkg := range server.Packages {
			if pkg.RegistryType == registryType {
				hasRegistryType = true
				break
			}
		}
		if !hasRegistryType {
			return false
		}
	}

	// Validation status filter
	if validationStatus != "" && server.ValidationStatus != validationStatus {
		return false
	}

	return true
}

// handleRegistryByID handles requests for specific registry entries
func (s *Service) handleRegistryByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/registry/")
	if id == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetRegistryByID(w, r, id)
	case http.MethodPut:
		s.handleUpdateRegistry(w, r, id)
	case http.MethodDelete:
		s.handleDeleteRegistry(w, r, id)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetRegistryByID gets a registry entry by ID
func (s *Service) handleGetRegistryByID(w http.ResponseWriter, r *http.Request, id string) {
	server, err := s.store.GetMCPServer(id)
	if err != nil {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(server)
}

// handleUpdateRegistry updates a registry entry
func (s *Service) handleUpdateRegistry(w http.ResponseWriter, r *http.Request, id string) {
	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	server, err := s.store.GetMCPServer(id)
	if err != nil {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	// Apply updates
	if name, ok := updates["name"].(string); ok {
		server.Name = name
	}
	if description, ok := updates["description"].(string); ok {
		server.Description = description
	}
	if validationStatus, ok := updates["validation_status"].(string); ok {
		server.ValidationStatus = validationStatus
	}

	if err := s.store.UpdateMCPServer(id, server); err != nil {
		http.Error(w, "Failed to update server", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(server)
}

// handleDeleteRegistry deletes a registry entry
func (s *Service) handleDeleteRegistry(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.store.DeleteMCPServer(id); err != nil {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleUpload handles single server upload
func (s *Service) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var server models.MCPServer
	if err := json.NewDecoder(r.Body).Decode(&server); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Set defaults
	if server.ID == "" {
		server.ID = fmt.Sprintf("custom-%d", time.Now().Unix())
	}
	if server.ValidationStatus == "" {
		server.ValidationStatus = "new"
	}
	server.DiscoveredAt = time.Now()

	if server.Meta == nil {
		server.Meta = make(map[string]interface{})
	}
	server.Meta["source"] = "custom"

	if err := s.store.CreateMCPServer(&server); err != nil {
		http.Error(w, "Failed to create server", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(server)
}

// handleBulkUpload handles bulk server upload
func (s *Service) handleBulkUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var servers []models.MCPServer
	if err := json.NewDecoder(r.Body).Decode(&servers); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	var created []models.MCPServer
	for i := range servers {
		server := &servers[i]

		// Set defaults
		if server.ID == "" {
			server.ID = fmt.Sprintf("custom-%d-%d", time.Now().Unix(), i)
		}
		if server.ValidationStatus == "" {
			server.ValidationStatus = "new"
		}
		server.DiscoveredAt = time.Now()

		if server.Meta == nil {
			server.Meta = make(map[string]interface{})
		}
		server.Meta["source"] = "custom"

		if err := s.store.CreateMCPServer(server); err != nil {
			log.Printf("Failed to create server %s: %v", server.ID, err)
			continue
		}

		created = append(created, *server)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

// handleSyncOfficial triggers official registry sync
func (s *Service) handleSyncOfficial(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	go func() {
		if err := s.syncOfficialRegistry(); err != nil {
			log.Printf("Official registry sync failed: %v", err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "sync_started",
		"source": "official",
	})
}

// handleSyncDocker triggers Docker registry sync
func (s *Service) handleSyncDocker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	go func() {
		if err := s.syncDockerRegistry(); err != nil {
			log.Printf("Docker registry sync failed: %v", err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "sync_started",
		"source": "docker",
	})
}
