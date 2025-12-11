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
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"suse-ai-up/internal/handlers"
	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/middleware"
	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/proxy"
	"suse-ai-up/pkg/services"

	adaptersvc "suse-ai-up/pkg/services/adapters"
)

// Service represents the registry service
type Service struct {
	config                 *Config
	server                 *http.Server
	store                  clients.MCPServerStore
	adapterStore           clients.AdapterResourceStore
	adapterService         *adaptersvc.AdapterService
	userStore              clients.UserStore
	groupStore             clients.GroupStore
	userGroupService       *services.UserGroupService
	userGroupHandler       *handlers.UserGroupHandler
	routeAssignmentHandler *handlers.RouteAssignmentHandler
	syncManager            *SyncManager
	sidecarManager         *proxy.SidecarManager
	shutdownCh             chan struct{}
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

// GetMCPServer gets an MCP server by ID (implements RegistryStore interface)
func (s *Service) GetMCPServer(id string) (*models.MCPServer, error) {
	return s.store.GetMCPServer(id)
}

// UpdateMCPServer updates an MCP server (implements RegistryStore interface)
func (s *Service) UpdateMCPServer(id string, updated *models.MCPServer) error {
	return s.store.UpdateMCPServer(id, updated)
}

// NewService creates a new registry service
func NewService(config *Config) *Service {
	// Initialize Kubernetes client and SidecarManager
	var sidecarManager *proxy.SidecarManager
	log.Printf("Initializing SidecarManager...")
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Printf("Failed to get in-cluster config: %v", err)
		kubeconfigPath := os.Getenv("KUBECONFIG")
		log.Printf("Trying kubeconfig from KUBECONFIG env var: %s", kubeconfigPath)
		if kubeconfigPath == "" {
			kubeconfigPath = "/Users/alessandrofesta/.lima/rancher/copied-from-guest/kubeconfig.yaml"
			log.Printf("KUBECONFIG not set, trying default path: %s", kubeconfigPath)
		}
		// Try to load from kubeconfig file
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			log.Printf("Failed to get Kubernetes config from file: %v", err)
			log.Printf("Sidecar functionality will not be available")
		} else {
			log.Printf("Successfully loaded kubeconfig from: %s", kubeconfigPath)
		}
	} else {
		log.Printf("Successfully loaded in-cluster config")
	}

	if kubeConfig != nil {
		kubeClient, err := kubernetes.NewForConfig(kubeConfig)
		if err != nil {
			log.Printf("Failed to create Kubernetes client: %v", err)
		} else {
			sidecarManager = proxy.NewSidecarManager(kubeClient, "default")
			log.Printf("SidecarManager initialized successfully")
		}
	}

	service := &Service{
		config:         config,
		store:          clients.NewInMemoryMCPServerStore(),
		adapterStore:   clients.NewInMemoryAdapterStore(),
		userStore:      clients.NewInMemoryUserStore(),
		groupStore:     clients.NewInMemoryGroupStore(),
		sidecarManager: sidecarManager,
		shutdownCh:     make(chan struct{}),
	}

	// Initialize user/group service
	service.userGroupService = services.NewUserGroupService(service.userStore, service.groupStore)

	// Initialize handlers
	service.userGroupHandler = handlers.NewUserGroupHandler(service.userGroupService)
	service.routeAssignmentHandler = handlers.NewRouteAssignmentHandler(service.userGroupService, service)

	// Initialize sync manager
	service.syncManager = NewSyncManager(service.store)

	// Initialize adapter service (sidecar manager will be set later if available)
	service.adapterService = adaptersvc.NewAdapterService(service.adapterStore, service.store, service.sidecarManager)

	return service
}

// Start starts the registry service
func (s *Service) Start() error {
	log.Printf("Starting MCP Registry service on port %d", s.config.Port)

	// Setup HTTP routes with CORS middleware
	mux := http.NewServeMux()
	mux.HandleFunc("/health", middleware.CORSMiddleware(s.handleHealth))

	// Swagger UI and JSON endpoints for registry service
	mux.HandleFunc("/docs", middleware.CORSMiddleware(s.handleRegistryDocs))
	mux.HandleFunc("/swagger.json", middleware.CORSMiddleware(s.handleRegistrySwaggerJSON))
	mux.HandleFunc("/api/v1/registry/browse", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(s.handleBrowse)))
	mux.HandleFunc("/api/v1/registry/", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(s.handleRegistryByID)))
	mux.HandleFunc("/api/v1/registry/upload", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(s.handleUpload)))
	mux.HandleFunc("/api/v1/registry/upload/bulk", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(s.handleBulkUpload)))
	mux.HandleFunc("/api/v1/registry/reload", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(s.handleReloadRemoteServers)))

	// Adapter management routes
	adapterHandler := handlers.NewAdapterHandler(s.adapterService)
	mux.HandleFunc("/api/v1/adapters", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(adapterHandler.HandleAdapters)))
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
			if len(parts) > 1 && parts[1] == "mcp" {
				// Handle MCP protocol requests - proxy to sidecar
				r.URL.Path = "/api/v1/adapters/" + adapterID + "/mcp"
				adapterHandler.HandleMCPProtocol(w, r)
			} else {
				// Regular GET request for adapter info
				r.URL.Path = "/api/v1/adapters/" + adapterID
				adapterHandler.GetAdapter(w, r)
			}
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
			} else if len(parts) > 1 && parts[1] == "mcp" {
				// Handle MCP protocol POST requests - proxy to sidecar
				r.URL.Path = "/api/v1/adapters/" + adapterID + "/mcp"
				adapterHandler.HandleMCPProtocol(w, r)
			} else {
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	})))

	// User and group management routes
	mux.HandleFunc("/api/v1/users", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(s.userGroupHandler.HandleUsers)))
	mux.HandleFunc("/api/v1/users/", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
		if path == "" {
			if r.Method == "GET" {
				s.userGroupHandler.ListUsers(w, r)
			} else {
				http.NotFound(w, r)
			}
			return
		}

		// Extract user ID from path
		userID := strings.Split(path, "/")[0]

		switch r.Method {
		case "GET":
			r.URL.Path = "/api/v1/users/" + userID
			s.userGroupHandler.GetUser(w, r)
		case "PUT":
			r.URL.Path = "/api/v1/users/" + userID
			s.userGroupHandler.UpdateUser(w, r)
		case "DELETE":
			r.URL.Path = "/api/v1/users/" + userID
			s.userGroupHandler.DeleteUser(w, r)
		default:
			http.NotFound(w, r)
		}
	})))

	// Group management routes
	mux.HandleFunc("/api/v1/groups", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(s.userGroupHandler.HandleGroups)))
	mux.HandleFunc("/api/v1/groups/", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/groups/")
		if path == "" {
			if r.Method == "GET" {
				s.userGroupHandler.ListGroups(w, r)
			} else {
				http.NotFound(w, r)
			}
			return
		}

		// Extract group ID from path
		parts := strings.Split(path, "/")
		groupID := parts[0]

		switch r.Method {
		case "GET":
			r.URL.Path = "/api/v1/groups/" + groupID
			s.userGroupHandler.GetGroup(w, r)
		case "PUT":
			r.URL.Path = "/api/v1/groups/" + groupID
			s.userGroupHandler.UpdateGroup(w, r)
		case "DELETE":
			r.URL.Path = "/api/v1/groups/" + groupID
			s.userGroupHandler.DeleteGroup(w, r)
		case "POST":
			if len(parts) > 1 && parts[1] == "members" {
				r.URL.Path = "/api/v1/groups/" + groupID + "/members"
				s.userGroupHandler.AddUserToGroup(w, r)
			} else {
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	})))

	// Load comprehensive MCP servers from curated list
	if err := s.loadComprehensiveServers(); err != nil {
		log.Printf("Failed to load comprehensive servers: %v", err)
		// Fall back to loading remote servers
		if err := s.loadRemoteServers(); err != nil {
			log.Printf("Failed to load remote servers: %v", err)
			// Continue anyway - service can still function
		}
	}

	// Start HTTP server
	httpServer := &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", s.config.Port),
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
				Addr:      fmt.Sprintf("0.0.0.0:%d", s.config.TLSPort),
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

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigChan:
		log.Println("Received shutdown signal")
	case <-s.shutdownCh:
		log.Println("Received internal shutdown signal")
	}

	return s.Stop()
}

// Stop stops the registry service
func (s *Service) Stop() error {
	log.Println("Stopping MCP Registry service")
	close(s.shutdownCh)
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

// loadComprehensiveServers loads comprehensive MCP servers from the curated JSON file
func (s *Service) loadComprehensiveServers() error {
	log.Println("Loading comprehensive MCP servers from curated list")

	comprehensiveFile := "config/comprehensive_mcp_servers.json"
	data, err := os.ReadFile(comprehensiveFile)
	if err != nil {
		return fmt.Errorf("failed to read comprehensive servers file %s: %w", comprehensiveFile, err)
	}

	var servers []models.MCPServer
	if err := json.Unmarshal(data, &servers); err != nil {
		return fmt.Errorf("failed to parse comprehensive servers JSON: %w", err)
	}

	// Store servers in the registry
	for _, server := range servers {
		// Ensure server has required fields
		if server.ID == "" {
			server.ID = fmt.Sprintf("comprehensive-%s", strings.ToLower(strings.ReplaceAll(server.Name, " ", "-")))
		}
		if server.ValidationStatus == "" {
			server.ValidationStatus = "approved"
		}
		server.DiscoveredAt = time.Now()

		// Mark these as loaded from comprehensive list
		if server.Meta == nil {
			server.Meta = make(map[string]interface{})
		}
		server.Meta["source"] = "comprehensive"

		if err := s.store.CreateMCPServer(&server); err != nil {
			log.Printf("Failed to store comprehensive server %s: %v", server.ID, err)
			// Continue with other servers
		}
	}

	log.Printf("Successfully loaded %d comprehensive MCP servers", len(servers))
	return nil
}

// loadRemoteServers loads remote MCP servers from the static JSON file
func (s *Service) loadRemoteServers() error {
	log.Println("Loading remote MCP servers from static file")

	// Try to read from the primary location first
	data, err := os.ReadFile(s.config.RemoteServersFile)
	if err != nil && os.IsNotExist(err) {
		// If primary file doesn't exist, try the fallback location
		userHomeDir, homeErr := os.UserHomeDir()
		if homeErr == nil {
			fallbackFile := userHomeDir + "/synced_servers/remote_mcp_servers.json"
			data, err = os.ReadFile(fallbackFile)
			if err != nil && os.IsNotExist(err) {
				log.Printf("Remote servers files not found at %s or %s, using in-memory storage only", s.config.RemoteServersFile, fallbackFile)
				return nil
			} else if err != nil {
				log.Printf("Failed to read fallback remote servers file %s: %v", fallbackFile, err)
				return nil // Don't fail, just use in-memory storage
			}
			log.Printf("Loading remote servers from fallback location: %s", fallbackFile)
		} else {
			log.Printf("Remote servers file %s not found and could not determine fallback location, using in-memory storage only", s.config.RemoteServersFile)
			return nil
		}
	} else if err != nil {
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

		// Mark these as loaded from file
		if server.Meta == nil {
			server.Meta = make(map[string]interface{})
		}
		server.Meta["source"] = "file"

		if err := s.store.CreateMCPServer(&server); err != nil {
			log.Printf("Failed to store remote server %s: %v", server.ID, err)
			// Continue with other servers
		}
	}

	log.Printf("Successfully loaded %d remote MCP servers", len(servers))
	return nil
}

// saveRemoteServers saves the current remote servers to the static file
func (s *Service) saveRemoteServers() error {
	// Get all servers from the store
	servers := s.store.ListMCPServers()

	// Filter to only remote servers (those with source metadata)
	var remoteServers []models.MCPServer
	for _, server := range servers {
		if server.Meta != nil {
			if source, ok := server.Meta["source"].(string); ok && source != "" && source != "file" {
				remoteServers = append(remoteServers, *server)
			}
		}
	}

	// Save to file
	data, err := json.MarshalIndent(remoteServers, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal servers: %w", err)
	}

	// Try to save to the configured file first
	if err := os.WriteFile(s.config.RemoteServersFile, data, 0644); err != nil {
		// If that fails, try to save to a user-writable location
		userHomeDir, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return fmt.Errorf("failed to write servers file to %s: %w, and could not get user home dir: %v", s.config.RemoteServersFile, err, homeErr)
		}

		// Create a synced_servers directory in user home
		syncedDir := userHomeDir + "/synced_servers"
		if mkdirErr := os.MkdirAll(syncedDir, 0755); mkdirErr != nil {
			return fmt.Errorf("failed to write servers file to %s: %w, and could not create synced dir %s: %v", s.config.RemoteServersFile, err, syncedDir, mkdirErr)
		}

		syncedFile := syncedDir + "/remote_mcp_servers.json"
		if writeErr := os.WriteFile(syncedFile, data, 0644); writeErr != nil {
			return fmt.Errorf("failed to write servers file to %s: %w, and also failed to write to %s: %v", s.config.RemoteServersFile, err, syncedFile, writeErr)
		}

		log.Printf("Saved %d remote servers to %s (fallback location)", len(remoteServers), syncedFile)
		return nil
	}

	log.Printf("Saved %d remote servers to %s", len(remoteServers), s.config.RemoteServersFile)
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
	category := query.Get("category")
	transportType := query.Get("transport")
	registryType := query.Get("registryType")
	validationStatus := query.Get("validationStatus")

	// Get all servers
	servers := s.store.ListMCPServers()

	// Sort by priority (SUSE servers first), then by name
	sort.Slice(servers, func(i, j int) bool {
		priorityI := 0
		priorityJ := 0

		if servers[i].Meta != nil {
			if p, ok := servers[i].Meta["priority"].(float64); ok {
				priorityI = int(p)
			}
		}
		if servers[j].Meta != nil {
			if p, ok := servers[j].Meta["priority"].(float64); ok {
				priorityJ = int(p)
			}
		}

		// Higher priority first, then alphabetical by name
		if priorityI != priorityJ {
			return priorityI > priorityJ
		}
		return strings.ToLower(servers[i].Name) < strings.ToLower(servers[j].Name)
	})

	// Apply filters
	var filtered []*models.MCPServer
	for _, server := range servers {
		if s.matchesFilters(server, searchQuery, category, transportType, registryType, validationStatus) {
			filtered = append(filtered, server)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(filtered)
}

// matchesFilters checks if a server matches the given filters
func (s *Service) matchesFilters(server *models.MCPServer, query, category, transport, registryType, validationStatus string) bool {
	// Search query filter
	if query != "" {
		queryLower := strings.ToLower(query)
		if !strings.Contains(strings.ToLower(server.Name), queryLower) &&
			!strings.Contains(strings.ToLower(server.Description), queryLower) {
			return false
		}
	}

	// Category filter
	if category != "" && server.Meta != nil {
		if metaCategory, ok := server.Meta["category"].(string); !ok || metaCategory != category {
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

// handleRegistryDocs serves the Swagger UI for registry service
func (s *Service) handleRegistryDocs(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>SUSE AI Universal Proxy - Registry API</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@3.25.0/swagger-ui.css" />
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@3.25.0/swagger-ui-bundle.js"></script>
    <script src="https://unpkg.com/swagger-ui-dist@3.25.0/swagger-ui-standalone-preset.js"></script>
    <script>
        window.onload = function() {
            const ui = SwaggerUIBundle({
                url: '/swagger.json',
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                plugins: [
                    SwaggerUIBundle.plugins.DownloadUrl
                ],
                layout: "StandaloneLayout"
            });
        };
    </script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// handleRegistrySwaggerJSON serves the Swagger JSON for registry service
func (s *Service) handleRegistrySwaggerJSON(w http.ResponseWriter, r *http.Request) {
	// Determine the host for registry service (port 8913)
	host := r.Host
	if host == "" {
		host = "192.168.64.17:8913"
	} else {
		// Replace port with 8913
		hostParts := strings.Split(host, ":")
		host = hostParts[0] + ":8913"
	}

	log.Printf("Registry Swagger requested from host: %s, setting host to: %s", r.Host, host)

	// Create a minimal swagger spec focused on registry operations
	swagger := map[string]interface{}{
		"swagger": "2.0",
		"info": map[string]interface{}{
			"title":       "SUSE AI Universal Proxy - Registry API",
			"description": "Registry service APIs for server management, adapters, and user/group management (Port 8913)",
			"version":     "1.0.0",
		},
		"host":     host,
		"basePath": "/",
		"schemes":  []string{"http", "https"},
		"consumes": []string{"application/json"},
		"produces": []string{"application/json"},
		"tags": []map[string]interface{}{
			{"name": "Registry", "description": "MCP server registry management"},
			{"name": "Adapters", "description": "Adapter management"},
			{"name": "User Management", "description": "User account management"},
			{"name": "Group Management", "description": "User group management"},
			{"name": "Route Management", "description": "Server access route assignments"},
		},
		"paths": map[string]interface{}{
			"/api/v1/registry/browse": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Registry"},
					"summary":     "Browse MCP Servers",
					"description": "Get a list of all available MCP servers in the registry",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of MCP servers",
							"schema": map[string]interface{}{
								"type": "array",
								"items": map[string]interface{}{
									"$ref": "#/definitions/MCPServer",
								},
							},
						},
					},
				},
			},
			"/api/v1/adapters": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Adapters"},
					"summary":     "List Adapters",
					"description": "Get a list of all adapters for the current user",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of adapters",
							"schema": map[string]interface{}{
								"type": "array",
								"items": map[string]interface{}{
									"type": "object",
								},
							},
						},
					},
				},
				"post": map[string]interface{}{
					"tags":        []string{"Adapters"},
					"summary":     "Create Adapter",
					"description": "Create a new adapter from a registry server",
					"parameters": []map[string]interface{}{
						{
							"name": "adapter",
							"in":   "body",
							"schema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"mcpServerId": map[string]interface{}{"type": "string"},
									"name":        map[string]interface{}{"type": "string"},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{
							"description": "Adapter created",
						},
					},
				},
			},
			"/api/v1/users": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"User Management"},
					"summary":     "List Users",
					"description": "Get a list of all users",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of users",
						},
					},
				},
				"post": map[string]interface{}{
					"tags":        []string{"User Management"},
					"summary":     "Create User",
					"description": "Create a new user",
					"responses": map[string]interface{}{
						"201": map[string]interface{}{
							"description": "User created",
						},
					},
				},
			},
			"/api/v1/groups": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Group Management"},
					"summary":     "List Groups",
					"description": "Get a list of all groups",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of groups",
						},
					},
				},
				"post": map[string]interface{}{
					"tags":        []string{"Group Management"},
					"summary":     "Create Group",
					"description": "Create a new group",
					"responses": map[string]interface{}{
						"201": map[string]interface{}{
							"description": "Group created",
						},
					},
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(swagger); err != nil {
		log.Printf("Failed to encode registry swagger JSON: %v", err)
		http.Error(w, "Failed to generate Swagger documentation", http.StatusInternalServerError)
	}
}

// handleRegistryByID handles requests for specific registry entries and route assignments
func (s *Service) handleRegistryByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/registry/")
	if path == "" {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(path, "/")
	if len(parts) >= 2 && parts[1] == "routes" {
		// Handle route assignment routes
		serverID := parts[0]
		switch r.Method {
		case http.MethodGet:
			r.URL.Path = "/api/v1/registry/" + serverID + "/routes"
			s.routeAssignmentHandler.ListRouteAssignments(w, r)
		case http.MethodPost:
			r.URL.Path = "/api/v1/registry/" + serverID + "/routes"
			s.routeAssignmentHandler.CreateRouteAssignment(w, r)
		case http.MethodPut:
			if len(parts) >= 3 {
				r.URL.Path = "/api/v1/registry/" + serverID + "/routes/" + parts[2]
				s.routeAssignmentHandler.UpdateRouteAssignment(w, r)
			} else {
				http.NotFound(w, r)
			}
		case http.MethodDelete:
			if len(parts) >= 3 {
				r.URL.Path = "/api/v1/registry/" + serverID + "/routes/" + parts[2]
				s.routeAssignmentHandler.DeleteRouteAssignment(w, r)
			} else {
				http.NotFound(w, r)
			}
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	// Handle regular registry routes
	id := parts[0]
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

// handleReloadRemoteServers reloads MCP servers from comprehensive list
func (s *Service) handleReloadRemoteServers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Clear existing servers before reloading
	log.Println("Clearing existing MCP servers before reload")
	existingServers := s.store.ListMCPServers()
	for _, server := range existingServers {
		if err := s.store.DeleteMCPServer(server.ID); err != nil {
			log.Printf("Failed to delete server %s during reload: %v", server.ID, err)
			// Continue with other servers
		}
	}
	log.Printf("Cleared %d existing servers", len(existingServers))

	// Reload from comprehensive list
	if err := s.loadComprehensiveServers(); err != nil {
		log.Printf("Failed to reload comprehensive servers: %v", err)
		http.Error(w, "Failed to reload MCP servers", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "reload_completed",
		"message": "MCP servers reloaded successfully from comprehensive list",
	})
}
