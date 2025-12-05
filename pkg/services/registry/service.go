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
	"os"
	"strings"
	"sync"
	"time"

	"suse-ai-up/internal/handlers"
	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/middleware"
	"suse-ai-up/pkg/models"

	adaptersvc "suse-ai-up/pkg/services/adapters"
)

// Service represents the registry service
type Service struct {
	config         *Config
	server         *http.Server
	store          clients.MCPServerStore
	adapterStore   clients.AdapterResourceStore
	adapterService *adaptersvc.AdapterService
	mu             sync.RWMutex
}

// Config holds registry service configuration
type Config struct {
	Port              int    `json:"port"`
	TLSPort           int    `json:"tls_port"`
	ConfigFile        string `json:"config_file"`
	RemoteServersFile string `json:"remote_servers_file"`
	AutoTLS           bool   `json:"auto_tls"`
	CertFile          string `json:"cert_file"`
	KeyFile           string `json:"key_file"`
}

// NewService creates a new registry service
func NewService(config *Config) *Service {
	if config.RemoteServersFile == "" {
		config.RemoteServersFile = "config/remote_mcp_servers.json"
	}

	service := &Service{
		config:       config,
		store:        clients.NewInMemoryMCPServerStore(),
		adapterStore: clients.NewFileAdapterStore("data/adapters.json"),
	}

	// Initialize adapter service
	service.adapterService = adaptersvc.NewAdapterService(service.adapterStore, service.store)

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
	mux.HandleFunc("/api/v1/registry/reload", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(s.handleReloadRemoteServers)))

	// Adapter management routes
	adapterHandler := handlers.NewAdapterHandler(s.adapterService)
	mux.HandleFunc("/api/v1/adapters", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(adapterHandler.CreateAdapter)))
	mux.HandleFunc("/api/v1/adapters/", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			adapterHandler.ListAdapters(w, r)
		} else {
			http.NotFound(w, r)
		}
	})))
	mux.HandleFunc("/api/v1/adapters/", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/adapters/")
		if path == "" {
			if r.Method == "GET" {
				adapterHandler.ListAdapters(w, r)
			} else {
				http.NotFound(w, r)
			}
			return
		}

		// Extract adapter ID from path
		parts := strings.Split(path, "/")
		if len(parts) == 0 {
			http.NotFound(w, r)
			return
		}

		adapterID := parts[0]

		switch r.Method {
		case "GET":
			// Set adapter ID in URL params for handler
			r.URL.Path = "/api/v1/adapters/" + adapterID
			adapterHandler.GetAdapter(w, r)
		case "PUT":
			r.URL.Path = "/api/v1/adapters/" + adapterID
			adapterHandler.UpdateAdapter(w, r)
		case "DELETE":
			r.URL.Path = "/api/v1/adapters/" + adapterID
			adapterHandler.DeleteAdapter(w, r)
		case "POST":
			if len(parts) > 1 && parts[1] == "sync" {
				r.URL.Path = "/api/v1/adapters/" + adapterID + "/sync"
				adapterHandler.SyncAdapterCapabilities(w, r)
			} else {
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	})))

	// Load remote MCP servers from static file
	if err := s.loadRemoteServers(); err != nil {
		log.Printf("Failed to load remote servers: %v", err)
		// Continue anyway - service can still function
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

// loadRemoteServers loads remote MCP servers from the static JSON file
func (s *Service) loadRemoteServers() error {
	log.Println("Loading remote MCP servers from static file")

	data, err := os.ReadFile(s.config.RemoteServersFile)
	if err != nil {
		return fmt.Errorf("failed to read remote servers file %s: %w", s.config.RemoteServersFile, err)
	}

	var servers []models.MCPServer
	if err := json.Unmarshal(data, &servers); err != nil {
		return fmt.Errorf("failed to parse remote servers JSON: %w", err)
	}

	// Store servers in the registry
	for _, server := range servers {
		// Ensure server has required fields
		if server.ID == "" {
			server.ID = fmt.Sprintf("remote-%s", strings.ToLower(strings.ReplaceAll(server.Name, " ", "-")))
		}
		if server.ValidationStatus == "" {
			server.ValidationStatus = "approved"
		}
		server.DiscoveredAt = time.Now()

		if err := s.store.CreateMCPServer(&server); err != nil {
			log.Printf("Failed to store remote server %s: %v", server.ID, err)
			// Continue with other servers
		}
	}

	log.Printf("Successfully loaded %d remote MCP servers", len(servers))
	return nil
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

// handleReloadRemoteServers reloads remote MCP servers from the static file
func (s *Service) handleReloadRemoteServers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Clear existing remote servers
	// Note: In a real implementation, you might want to be more selective
	// For now, we'll reload all servers

	if err := s.loadRemoteServers(); err != nil {
		log.Printf("Failed to reload remote servers: %v", err)
		http.Error(w, "Failed to reload remote servers", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "reload_completed",
		"message": "Remote MCP servers reloaded successfully",
	})
}
