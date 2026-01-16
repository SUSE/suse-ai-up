package registry

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
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

	"gopkg.in/yaml.v3"
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

	// Load servers from YAML file if configured
	if s.config.RemoteServersFile != "" {
		if err := s.loadServersFromFile(s.config.RemoteServersFile); err != nil {
			log.Printf("Warning: Failed to load servers from %s: %v", s.config.RemoteServersFile, err)
		} else {
			log.Printf("Successfully loaded servers from %s", s.config.RemoteServersFile)
		}
	}

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
	adapterHandler := handlers.NewAdapterHandler(s.adapterService, s.userGroupService)
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
		groupID := strings.Split(path, "/")[0]

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
		default:
			http.NotFound(w, r)
		}
	})))

	// Start HTTP server
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.Port),
		Handler: mux,
	}

	// Start server
	log.Printf("Registry HTTP server listening on :%d", s.config.Port)
	return server.ListenAndServe()
}

// loadServersFromFile loads MCP servers from a YAML or JSON file
func (s *Service) loadServersFromFile(filePath string) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", filePath)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	var servers []models.MCPServer

	// Try YAML first
	if strings.HasSuffix(filePath, ".yaml") || strings.HasSuffix(filePath, ".yml") {
		if err := yaml.Unmarshal(data, &servers); err != nil {
			return fmt.Errorf("failed to parse YAML: %w", err)
		}
	} else {
		// Try JSON
		if err := json.Unmarshal(data, &servers); err != nil {
			return fmt.Errorf("failed to parse JSON: %w", err)
		}
	}

	// Store servers
	for _, server := range servers {
		if err := s.store.CreateMCPServer(&server); err != nil {
			log.Printf("Warning: Failed to store server %s: %v", server.ID, err)
		}
	}

	log.Printf("Loaded %d servers from %s", len(servers), filePath)
	return nil
}

// startHTTPServer starts the HTTPS server (placeholder)
func (s *Service) startHTTPSServer() {
	// Placeholder for HTTPS server - not implemented yet
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

// handleRegistryDocs serves the Swagger UI
func (s *Service) handleRegistryDocs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
    <title>Registry API Documentation</title>
</head>
<body>
    <h1>Registry Service API</h1>
    <p>API documentation for the MCP Registry service.</p>
    <a href="/swagger.json">Swagger JSON</a>
</body>
</html>`))
}

// handleRegistrySwaggerJSON serves the Swagger JSON
func (s *Service) handleRegistrySwaggerJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	swagger := map[string]interface{}{
		"swagger": "2.0",
		"info": map[string]interface{}{
			"title":   "MCP Registry API",
			"version": "1.0.0",
		},
		"paths": map[string]interface{}{
			"/api/v1/registry/browse": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "Browse MCP servers",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of MCP servers",
						},
					},
				},
			},
		},
	}
	json.NewEncoder(w).Encode(swagger)
}

// handleBrowse returns a list of all MCP servers
func (s *Service) handleBrowse(w http.ResponseWriter, r *http.Request) {
	servers := s.store.ListMCPServers()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(servers)
}

// handleRegistryByID returns a specific MCP server by ID
func (s *Service) handleRegistryByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/registry/")
	server, err := s.store.GetMCPServer(id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Server not found"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(server)
}

// handleUpload uploads a single MCP server
func (s *Service) handleUpload(w http.ResponseWriter, r *http.Request) {
	var server models.MCPServer
	if err := json.NewDecoder(r.Body).Decode(&server); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON"})
		return
	}

	if err := s.store.CreateMCPServer(&server); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to store server"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// handleBulkUpload uploads multiple MCP servers
func (s *Service) handleBulkUpload(w http.ResponseWriter, r *http.Request) {
	var servers []models.MCPServer
	if err := json.NewDecoder(r.Body).Decode(&servers); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON"})
		return
	}

	for _, server := range servers {
		if err := s.store.CreateMCPServer(&server); err != nil {
			log.Printf("Warning: Failed to store server %s: %v", server.ID, err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"count":  len(servers),
	})
}

// handleReloadRemoteServers reloads servers from remote sources
func (s *Service) handleReloadRemoteServers(w http.ResponseWriter, r *http.Request) {
	log.Println("Reloading MCP servers from configuration files")

	// Clear existing servers (except those created via API)
	// For simplicity, we'll reload from the config file
	if s.config.RemoteServersFile != "" {
		if err := s.loadServersFromFile(s.config.RemoteServersFile); err != nil {
			log.Printf("Warning: Failed to reload servers from %s: %v", s.config.RemoteServersFile, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Failed to reload servers"})
			return
		} else {
			log.Printf("Successfully reloaded servers from %s", s.config.RemoteServersFile)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "reload completed"})
}
