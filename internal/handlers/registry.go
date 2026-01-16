package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"suse-ai-up/internal/config"
	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/mcp"
	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/services"
)

// parseAndUploadRegistryYAML parses YAML data and uploads MCP servers to registry
func parseAndUploadRegistryYAML(data []byte, registryManager RegistryManagerInterface, source string) error {
	var servers []map[string]interface{}
	if err := yaml.Unmarshal(data, &servers); err != nil {
		return fmt.Errorf("could not parse registry YAML from %s: %w", source, err)
	}

	log.Printf("Loading %d MCP servers from %s", len(servers), source)

	var mcpServers []*models.MCPServer
	for _, serverData := range servers {
		server := &models.MCPServer{}

		if name, ok := serverData["name"].(string); ok {
			server.ID = name
			server.Name = name
		} else {
			log.Printf("Warning: Server missing name field, skipping: %+v", serverData)
			continue
		}

		if desc, ok := serverData["description"].(string); ok {
			server.Description = desc
		}

		if image, ok := serverData["image"].(string); ok {
			server.Packages = []models.Package{
				{
					Identifier: image,
					Transport: models.Transport{
						Type: "stdio",
					},
				},
			}
		}

		// Handle meta field
		if meta, ok := serverData["meta"].(map[string]interface{}); ok {
			server.Meta = meta
		} else {
			server.Meta = make(map[string]interface{})
		}

		server.Meta["source"] = "yaml"

		// Include additional fields
		if about, ok := serverData["about"].(map[string]interface{}); ok {
			server.Meta["about"] = about
		}
		if sourceInfo, ok := serverData["source"].(map[string]interface{}); ok {
			server.Meta["source_info"] = sourceInfo
		}
		if configField, ok := serverData["config"].(map[string]interface{}); ok {
			server.Meta["config"] = configField
		}
		if serverType, ok := serverData["type"].(string); ok {
			server.Meta["type"] = serverType
		}

		mcpServers = append(mcpServers, server)
	}

	// Use the registry manager to upload all servers
	if err := registryManager.UploadRegistryEntries(mcpServers); err != nil {
		return fmt.Errorf("could not upload registry entries: %w", err)
	}

	return nil
}

// ServerType represents the type of MCP server
type ServerType string

const (
	ServerTypeLocalStdio ServerType = "localstdio"
	ServerTypeRemoteHTTP ServerType = "remotehttp"
	ServerTypeGitHub     ServerType = "github"
)

type RegistryManagerInterface interface {
	UploadRegistryEntries(entries []*models.MCPServer) error
	LoadFromCustomSource(sourceURL string) error
	SearchServers(query string, filters map[string]interface{}) ([]*models.MCPServer, error)
	Clear() error
}

// MCPServerStore interface for MCP server storage operations
type MCPServerStore interface {
	CreateMCPServer(server *models.MCPServer) error
	GetMCPServer(id string) (*models.MCPServer, error)
	UpdateMCPServer(id string, updated *models.MCPServer) error
	DeleteMCPServer(id string) error
	ListMCPServers() []*models.MCPServer
}

// RegistryHandler handles MCP server registry operations
type RegistryHandler struct {
	Store            MCPServerStore
	RegistryManager  RegistryManagerInterface
	AdapterStore     clients.AdapterResourceStore
	ToolDiscovery    *mcp.MCPToolDiscoveryService
	UserGroupService *services.UserGroupService
	Config           *config.Config
	K8sClient        kubernetes.Interface // Kubernetes client for ConfigMap updates
}

// NewRegistryHandler creates a new registry handler
func NewRegistryHandler(store MCPServerStore, registryManager RegistryManagerInterface, adapterStore clients.AdapterResourceStore, userGroupService *services.UserGroupService, cfg *config.Config, k8sClient kubernetes.Interface) *RegistryHandler {
	handler := &RegistryHandler{
		Store:            store,
		RegistryManager:  registryManager,
		AdapterStore:     adapterStore,
		ToolDiscovery:    mcp.NewMCPToolDiscoveryService(),
		UserGroupService: userGroupService,
		Config:           cfg,
		K8sClient:        k8sClient,
	}

	return handler
}

// updateRegistryConfigMap updates the Kubernetes ConfigMap with new registry data
func (h *RegistryHandler) updateRegistryConfigMap(ctx context.Context, registryData []byte) error {
	if h.K8sClient == nil {
		// Not running in Kubernetes, skip ConfigMap update
		return nil
	}

	// Get namespace from environment variables or service account token location
	namespace := os.Getenv("NAMESPACE")
	if namespace == "" {
		// Try to get namespace from service account token path
		if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
			namespace = string(data)
		} else {
			// Fallback to environment variables that might be set
			namespace = os.Getenv("POD_NAMESPACE")
			if namespace == "" {
				namespace = "default"
			}
		}
	}

	// Get ConfigMap name from environment or construct it
	configMapName := os.Getenv("REGISTRY_CONFIGMAP_NAME")
	if configMapName == "" {
		// Try to construct name based on common patterns
		deploymentName := os.Getenv("DEPLOYMENT_NAME")
		if deploymentName == "" {
			deploymentName = os.Getenv("HOSTNAME") // Might contain deployment info
			if deploymentName == "" {
				deploymentName = "suse-ai-up"
			}
		}
		configMapName = deploymentName + "-registry"
	}

	// Get the current ConfigMap
	configMap, err := h.K8sClient.CoreV1().ConfigMaps(namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		log.Printf("Warning: Failed to get ConfigMap %s/%s: %v", namespace, configMapName, err)
		return fmt.Errorf("failed to get ConfigMap: %w", err)
	}

	// Update the mcp_registry.yaml data
	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}
	configMap.Data["mcp_registry.yaml"] = string(registryData)

	// Update the ConfigMap
	_, err = h.K8sClient.CoreV1().ConfigMaps(namespace).Update(ctx, configMap, metav1.UpdateOptions{})
	if err != nil {
		log.Printf("Warning: Failed to update ConfigMap %s/%s: %v", namespace, configMapName, err)
		return fmt.Errorf("failed to update ConfigMap: %w", err)
	}

	log.Printf("Successfully updated ConfigMap %s/%s with new registry data", namespace, configMapName)
	return nil
}

// DetectServerType determines the type of MCP server from registry metadata and package information
func DetectServerType(server *models.MCPServer) ServerType {
	// Check for GitHub servers first
	if server.GitHubConfig != nil || (server.Meta != nil && server.Meta["source"] == "github") {
		return ServerTypeGitHub
	}

	// Check metadata source first
	if server.Meta != nil {
		if source, ok := server.Meta["source"].(string); ok {
			switch strings.ToLower(source) {
			case "localstdio", "stdio", "local":
				return ServerTypeLocalStdio
			case "remote", "remotehttp", "http":
				return ServerTypeRemoteHTTP
			}
		}
	}

	// Check package transport information
	if len(server.Packages) > 0 {
		transport := server.Packages[0].Transport.Type
		switch strings.ToLower(transport) {
		case "stdio":
			return ServerTypeLocalStdio
		case "http", "sse", "websocket":
			return ServerTypeRemoteHTTP
		}
	}

	// Check legacy URL field for remote servers
	if server.URL != "" {
		return ServerTypeRemoteHTTP
	}

	// Default to LocalStdio for backward compatibility
	return ServerTypeLocalStdio
}

// GetMCPServer handles GET /registry/{id}
// @Summary Get an MCP server by ID
// @Description Retrieve a specific MCP server configuration
// @Tags registry
// @Produce json
// @Param id path string true "MCP Server ID"
// @Success 200 {object} models.MCPServer
// @Failure 404 {string} string "Not Found"
// @Router /api/v1/registry/{id} [get]
func (h *RegistryHandler) GetMCPServer(c *gin.Context) {
	id := c.Param("id")
	server, err := h.Store.GetMCPServer(id)
	if err != nil {
		log.Printf("MCP server not found: %s", id)
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, server)
}

// UpdateMCPServer handles PUT /registry/{id}
// @Summary Update an MCP server
// @Description Update an existing MCP server configuration or validation status
// @Tags registry
// @Accept json
// @Produce json
// @Param id path string true "MCP Server ID"
// @Param server body models.MCPServer true "Updated MCP server data"
// @Success 200 {object} models.MCPServer
// @Failure 400 {string} string "Bad Request"
// @Failure 404 {string} string "Not Found"
// @Router /api/v1/registry/{id} [put]
func (h *RegistryHandler) UpdateMCPServer(c *gin.Context) {
	id := c.Param("id")
	var updated models.MCPServer
	if err := c.ShouldBindJSON(&updated); err != nil {
		log.Printf("Error decoding MCP server update: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.Store.UpdateMCPServer(id, &updated); err != nil {
		log.Printf("Error updating MCP server: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	log.Printf("Updated MCP server: %s", id)
	c.JSON(http.StatusOK, updated)
}

// DeleteMCPServer handles DELETE /registry/{id}
// @Summary Delete an MCP server
// @Description Remove an MCP server entry
// @Tags registry
// @Param id path string true "MCP Server ID"
// @Success 204 "No Content"
// @Failure 404 {string} string "Not Found"
// @Router /api/v1/registry/{id} [delete]
func (h *RegistryHandler) DeleteMCPServer(c *gin.Context) {
	id := c.Param("id")
	if err := h.Store.DeleteMCPServer(id); err != nil {
		log.Printf("Error deleting MCP server: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	log.Printf("Deleted MCP server: %s", id)
	c.Status(http.StatusNoContent)
}

// validateMCPServer checks if a URL is an MCP server by attempting to connect as an MCP client
// TODO: Implement MCP validation when MCP SDK is available
func (h *RegistryHandler) validateMCPServer(url string) bool {
	// Placeholder implementation
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("URL %s not reachable: %v", url, err)
		return false
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("URL %s returned status %d", url, resp.StatusCode)
		return false
	}
	return true
}

// enumerateTools attempts to get the list of tools from an MCP server
// TODO: Implement tool enumeration when MCP SDK is available
func (h *RegistryHandler) enumerateTools(url string) ([]models.MCPTool, error) {
	// Placeholder implementation
	return []models.MCPTool{}, nil
}

// ListMCPServersFiltered lists servers with permission-based filtering
func (h *RegistryHandler) ListMCPServersFiltered(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user"
	}

	allServers := h.Store.ListMCPServers()

	// Check if user is admin
	canSeeAll := false
	if h.UserGroupService != nil {
		if canManage, err := h.UserGroupService.CanManageGroups(r.Context(), userID); err == nil && canManage {
			canSeeAll = true
		}
	}

	if canSeeAll {
		// Admins see all servers
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(allServers)
		return
	}

	// Filter servers based on user permissions
	var filteredServers []*models.MCPServer
	for _, server := range allServers {
		if canAccess, _ := h.UserGroupService.CanAccessServer(r.Context(), userID, server.ID); canAccess {
			filteredServers = append(filteredServers, server)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(filteredServers)
}

// convertToMCPServer converts a map representation to MCPServer model
func (h *RegistryHandler) convertToMCPServer(serverMap map[string]interface{}, source string) *models.MCPServer {
	server := &models.MCPServer{
		ValidationStatus: "new",
		DiscoveredAt:     time.Now(),
		Meta:             make(map[string]interface{}),
	}

	// Extract server data from nested structure (v0.1 API format)
	serverData, hasServer := serverMap["server"].(map[string]interface{})
	if !hasServer {
		// Fall back to direct access for Docker registry format
		serverData = serverMap
	}

	// Extract basic fields
	if id, ok := serverData["name"].(string); ok {
		server.ID = id
	} else {
		// Generate ID if not present
		server.ID = generateID()
	}

	if name, ok := serverData["name"].(string); ok {
		server.Name = name
	}

	if desc, ok := serverData["description"].(string); ok {
		server.Description = desc
	}

	if desc, ok := serverData["description"].(string); ok {
		server.Description = desc
	}

	if version, ok := serverData["version"].(string); ok {
		server.Version = version
	}

	// Handle repository
	if repoData, ok := serverData["repository"].(map[string]interface{}); ok {
		repo := models.Repository{}
		if url, ok := repoData["url"].(string); ok {
			repo.URL = url
		}
		if src, ok := repoData["source"].(string); ok {
			repo.Source = src
		}
		server.Repository = repo
	}

	// Handle packages
	if packagesData, ok := serverData["packages"].([]interface{}); ok {
		var packages []models.Package
		for _, pkgData := range packagesData {
			if pkgMap, ok := pkgData.(map[string]interface{}); ok {
				pkg := models.Package{}
				if regType, ok := pkgMap["registryType"].(string); ok {
					pkg.RegistryType = regType
				}
				if identifier, ok := pkgMap["identifier"].(string); ok {
					pkg.Identifier = identifier
				}
				if transportData, ok := pkgMap["transport"].(map[string]interface{}); ok {
					if transportType, ok := transportData["type"].(string); ok {
						pkg.Transport = models.Transport{Type: transportType}
					}
				}
				packages = append(packages, pkg)
			}
		}
		server.Packages = packages
	} else if packagesData, ok := serverData["packages"].([]map[string]interface{}); ok {
		var packages []models.Package
		for _, pkgMap := range packagesData {
			pkg := models.Package{}
			if regType, ok := pkgMap["registryType"].(string); ok {
				pkg.RegistryType = regType
			}
			if identifier, ok := pkgMap["identifier"].(string); ok {
				pkg.Identifier = identifier
			}
			if transportData, ok := pkgMap["transport"].(map[string]interface{}); ok {
				if transportType, ok := transportData["type"].(string); ok {
					pkg.Transport = models.Transport{Type: transportType}
				}
			}
			packages = append(packages, pkg)
		}
		server.Packages = packages
	}

	// Handle remotes (alternative to packages in some servers)
	if remotesData, ok := serverData["remotes"].([]interface{}); ok {
		var packages []models.Package
		for _, remoteData := range remotesData {
			if remoteMap, ok := remoteData.(map[string]interface{}); ok {
				pkg := models.Package{}
				if remoteType, ok := remoteMap["type"].(string); ok {
					// Convert remote type to transport type
					switch remoteType {
					case "streamable-http":
						pkg.Transport = models.Transport{Type: "http"}
					case "sse":
						pkg.Transport = models.Transport{Type: "sse"}
					default:
						pkg.Transport = models.Transport{Type: remoteType}
					}
				}
				if url, ok := remoteMap["url"].(string); ok {
					pkg.Identifier = url
					pkg.RegistryType = "remote"
				}
				packages = append(packages, pkg)
			}
		}
		server.Packages = packages
	}

	// Handle tools
	if toolsData, ok := serverData["tools"].([]interface{}); ok {
		var tools []models.MCPTool
		for _, toolData := range toolsData {
			if toolMap, ok := toolData.(map[string]interface{}); ok {
				tool := models.MCPTool{}
				if name, ok := toolMap["name"].(string); ok {
					tool.Name = name
				}
				if desc, ok := toolMap["description"].(string); ok {
					tool.Description = desc
				}
				if schema, ok := toolMap["input_schema"].(map[string]interface{}); ok {
					tool.InputSchema = schema
				}
				tools = append(tools, tool)
			}
		}
		server.Tools = tools
	}

	// Handle meta
	if meta, ok := serverMap["_meta"].(map[string]interface{}); ok {
		server.Meta = meta
	}

	// Add source information to meta
	if server.Meta == nil {
		server.Meta = make(map[string]interface{})
	}
	server.Meta["source"] = source

	return server
}

// UploadRegistryEntry handles POST /registry/upload
// @Summary Upload a single registry entry
// @Description Upload a single MCP server registry entry
// @Tags registry
// @Accept json
// @Produce json
// @Param server body models.MCPServer true "MCP server data"
// @Success 201 {object} models.MCPServer
// @Failure 400 {string} string "Bad Request"
// @Router /api/v1/registry/upload [post]
func (h *RegistryHandler) UploadRegistryEntry(c *gin.Context) {
	var server models.MCPServer
	if err := c.ShouldBindJSON(&server); err != nil {
		log.Printf("Error decoding MCP server: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	// Validation
	if server.Name == "" {
		log.Printf("MCP server name is required")
		c.JSON(http.StatusBadRequest, gin.H{"error": "MCP server name is required"})
		return
	}

	if server.ID == "" {
		server.ID = generateID()
	}

	// Use RegistryManager to upload
	if err := h.RegistryManager.UploadRegistryEntries([]*models.MCPServer{&server}); err != nil {
		log.Printf("Error uploading MCP server: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("Uploaded MCP server: %s", server.ID)
	c.JSON(http.StatusCreated, server)
}

// UploadBulkRegistryEntries handles POST /registry/upload/bulk
// @Summary Upload multiple registry entries
// @Description Upload multiple MCP server registry entries in bulk
// @Tags registry
// @Accept json
// @Produce json
// @Param servers body []models.MCPServer true "Array of MCP server data"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {string} string "Bad Request"
// @Router /api/v1/registry/upload/bulk [post]
func (h *RegistryHandler) UploadBulkRegistryEntries(c *gin.Context) {
	var servers []*models.MCPServer
	if err := c.ShouldBindJSON(&servers); err != nil {
		log.Printf("Error decoding MCP servers: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	// Generate IDs for servers that don't have them
	for _, server := range servers {
		if server.ID == "" {
			server.ID = generateID()
		}
	}

	// Use RegistryManager to upload
	if err := h.RegistryManager.UploadRegistryEntries(servers); err != nil {
		log.Printf("Error uploading MCP servers: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := map[string]interface{}{
		"message": fmt.Sprintf("Successfully uploaded %d MCP servers", len(servers)),
		"count":   len(servers),
	}

	log.Printf("Bulk uploaded %d MCP servers", len(servers))
	c.JSON(http.StatusOK, response)
}

// ReloadRegistry handles POST /registry/reload
// @Summary Reload registry from configured source
// @Description Reload MCP server registry from URL or local file based on configuration
// @Tags registry
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {string} string "Internal Server Error"
// @Router /api/v1/registry/reload [post]
func (h *RegistryHandler) ReloadRegistry(c *gin.Context) {
	log.Printf("Reloading MCP registry")

	var source string
	var serverCount int
	var err error

	// Try to load from URL first if configured
	if h.Config.MCPRegistryURL != "" {
		timeout, parseErr := time.ParseDuration(h.Config.RegistryTimeout)
		if parseErr != nil {
			log.Printf("Warning: Invalid registry timeout %s, using 30s: %v", h.Config.RegistryTimeout, parseErr)
			timeout = 30 * time.Second
		}

		source = h.Config.MCPRegistryURL

		client := &http.Client{
			Timeout: timeout,
		}

		resp, httpErr := client.Get(source)
		if httpErr != nil {
			err = fmt.Errorf("failed to fetch from URL %s: %w", source, httpErr)
		} else {
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				err = fmt.Errorf("URL returned status %d", resp.StatusCode)
			} else {
				data, readErr := io.ReadAll(resp.Body)
				if readErr != nil {
					err = fmt.Errorf("failed to read response body: %w", readErr)
				} else {
					// Parse and upload the YAML
					var servers []map[string]interface{}
					if parseErr := json.Unmarshal(data, &servers); parseErr != nil {
						// Try YAML if JSON fails
						servers = nil
						if yamlErr := yaml.Unmarshal(data, &servers); yamlErr != nil {
							err = fmt.Errorf("could not parse registry data from %s as JSON or YAML: %w", source, yamlErr)
						}
					}

					if err == nil {
						log.Printf("Loading %d MCP servers from %s", len(servers), source)
						var mcpServers []*models.MCPServer

						for _, serverData := range servers {
							server := &models.MCPServer{}

							if name, ok := serverData["name"].(string); ok {
								server.ID = name
								server.Name = name
							} else {
								log.Printf("Warning: Server missing name field, skipping: %+v", serverData)
								continue
							}

							if desc, ok := serverData["description"].(string); ok {
								server.Description = desc
							}

							if image, ok := serverData["image"].(string); ok {
								server.Packages = []models.Package{
									{
										Identifier: image,
										Transport: models.Transport{
											Type: "stdio",
										},
									},
								}
							}

							// Handle meta field
							if meta, ok := serverData["meta"].(map[string]interface{}); ok {
								server.Meta = meta
							} else {
								server.Meta = make(map[string]interface{})
							}

							server.Meta["source"] = "yaml"

							// Include additional fields
							if about, ok := serverData["about"].(map[string]interface{}); ok {
								server.Meta["about"] = about
							}
							if sourceInfo, ok := serverData["source"].(map[string]interface{}); ok {
								server.Meta["source_info"] = sourceInfo
							}
							if configField, ok := serverData["config"].(map[string]interface{}); ok {
								server.Meta["config"] = configField
							}
							if serverType, ok := serverData["type"].(string); ok {
								server.Meta["type"] = serverType
							}

							mcpServers = append(mcpServers, server)
						}

						// Clear existing registry before uploading new entries
						if clearErr := h.RegistryManager.Clear(); clearErr != nil {
							log.Printf("Warning: Failed to clear registry before reload: %v", clearErr)
							// Continue anyway - better to have some servers than none
						}

						// Upload all servers
						if uploadErr := h.RegistryManager.UploadRegistryEntries(mcpServers); uploadErr != nil {
							err = fmt.Errorf("could not upload registry entries: %w", uploadErr)
						} else {
							serverCount = len(mcpServers)

							// Update ConfigMap with new registry data for persistence
							if updateErr := h.updateRegistryConfigMap(c.Request.Context(), data); updateErr != nil {
								log.Printf("Warning: Failed to update registry ConfigMap: %v", updateErr)
								// Don't fail the reload if ConfigMap update fails
							}
						}
					}
				}
			}
		}
	}

	// If no URL configured or URL failed, fall back to local file
	if source == "" || err != nil {
		source = "config/mcp_registry.yaml"
		log.Printf("Loading MCP registry from local file: %s", source)

		data, readErr := os.ReadFile(source)
		if readErr != nil {
			err = fmt.Errorf("failed to read local file %s: %w", source, readErr)
		} else {
			parseErr := parseAndUploadRegistryYAML(data, h.RegistryManager, source)
			if parseErr != nil {
				err = fmt.Errorf("failed to parse local file %s: %w", source, parseErr)
			} else {
				// Count servers in registry after loading
				serverCount = len(h.Store.ListMCPServers())
				// Clear any previous error since fallback succeeded
				err = nil
			}
		}
	}

	if err != nil {
		log.Printf("Error reloading registry: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := map[string]interface{}{
		"message":     "Registry reloaded successfully",
		"source":      source,
		"serverCount": serverCount,
	}

	log.Printf("Registry reloaded from %s with %d servers", source, serverCount)
	c.JSON(http.StatusOK, response)
}

// BrowseRegistry handles GET /registry/browse
// @Summary Browse registry servers with search and filters
// @Description Search and filter MCP servers from local YAML configuration
// @Tags registry
// @Produce json
// @Param q query string false "Search query"
// @Param transport query string false "Filter by transport type (stdio, sse, websocket)"
// @Param registryType query string false "Filter by registry type (oci, npm)"
// @Param validationStatus query string false "Filter by validation status"
// @Param source query string false "Filter by source (yaml)"
// @Success 200 {array} models.MCPServer
// @Router /api/v1/registry/browse [get]
func (h *RegistryHandler) BrowseRegistry(c *gin.Context) {
	// Get query parameters
	query := c.Query("q")
	transport := c.Query("transport")
	registryType := c.Query("registryType")
	validationStatus := c.Query("validationStatus")
	source := c.Query("source")

	log.Printf("BrowseRegistry called with query=%s, transport=%s, registryType=%s, validationStatus=%s, source=%s",
		query, transport, registryType, validationStatus, source)

	// Get all servers from the store
	allServers := h.Store.ListMCPServers()
	log.Printf("Found %d servers in registry", len(allServers))

	// Apply filters
	var filteredServers []*models.MCPServer

	for _, server := range allServers {
		// Apply search query filter
		if query != "" {
			if !strings.Contains(strings.ToLower(server.Name), strings.ToLower(query)) &&
				!strings.Contains(strings.ToLower(server.Description), strings.ToLower(query)) {
				continue
			}
		}

		// Apply transport filter
		if transport != "" {
			hasTransport := false
			for _, pkg := range server.Packages {
				if pkg.Transport.Type == transport {
					hasTransport = true
					break
				}
			}
			if !hasTransport {
				continue
			}
		}

		// Apply registry type filter
		if registryType != "" {
			hasRegistryType := false
			for _, pkg := range server.Packages {
				if pkg.RegistryType == registryType {
					hasRegistryType = true
					break
				}
			}
			if !hasRegistryType {
				continue
			}
		}

		// Apply validation status filter
		if validationStatus != "" && server.ValidationStatus != validationStatus {
			continue
		}

		// Apply source filter
		if source != "" {
			if server.Meta == nil || server.Meta["registry_source"] != source {
				continue
			}
		}

		filteredServers = append(filteredServers, server)
	}

	log.Printf("Filtered to %d servers", len(filteredServers))
	c.JSON(http.StatusOK, filteredServers)
}

// UploadLocalMCP handles POST /registry/upload/local-mcp
// @Summary Upload a local MCP server implementation
// @Description Upload Python scripts and configuration for a local STDIO MCP server
// @Tags registry
// @Accept multipart/form-data
// @Produce json
// @Param name formData string true "MCP server name"
// @Param description formData string false "MCP server description"
// @Param config formData string true "MCP client configuration JSON"
// @Param files formData []file true "Python script files and requirements.txt"
// @Success 201 {object} models.MCPServer
// @Failure 400 {string} string "Bad Request"
// @Router /api/v1/registry/upload/local-mcp [post]
func (h *RegistryHandler) UploadLocalMCP(c *gin.Context) {
	// Parse form data
	name := c.PostForm("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	description := c.PostForm("description")
	configStr := c.PostForm("config")
	if configStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "config is required"})
		return
	}

	// Parse MCP client config
	var mcpConfig models.MCPClientConfig
	if err := json.Unmarshal([]byte(configStr), &mcpConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid MCP client configuration JSON"})
		return
	}

	// Validate config has at least one server
	if len(mcpConfig.MCPServers) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "MCP client config must contain at least one server"})
		return
	}

	// Get uploaded files
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse multipart form"})
		return
	}

	files := form.File["files"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least one file must be uploaded"})
		return
	}

	// Validate file types and store files
	// For now, we'll store in memory - in production, you'd want persistent storage
	fileContents := make(map[string][]byte)
	for _, fileHeader := range files {
		filename := fileHeader.Filename

		// Basic validation
		if !isValidMCPFile(filename) {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid file type: %s", filename)})
			return
		}

		file, err := fileHeader.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open uploaded file"})
			return
		}
		defer file.Close()

		content, err := io.ReadAll(file)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read uploaded file"})
			return
		}

		fileContents[filename] = content
	}

	// Create MCPServer entry
	serverID := generateID()
	server := &models.MCPServer{
		ID:               serverID,
		Name:             name,
		Description:      description,
		ValidationStatus: "uploaded",
		DiscoveredAt:     time.Now(),
		Meta: map[string]interface{}{
			"isLocalMCP":      true,
			"mcpClientConfig": mcpConfig,
			"uploadedFiles":   fileContents, // In production, store files separately
		},
	}

	// Add package info for the first server in the config
	for serverName := range mcpConfig.MCPServers {
		server.Packages = []models.Package{
			{
				RegistryType: "local",
				Identifier:   serverName,
				Transport: models.Transport{
					Type: "stdio",
				},
			},
		}
		break // Only use the first server for now
	}

	// Store in registry
	if err := h.Store.CreateMCPServer(server); err != nil {
		log.Printf("Error storing local MCP server: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store MCP server"})
		return
	}

	log.Printf("Uploaded local MCP server: %s", serverID)
	c.JSON(http.StatusCreated, server)
}

// isValidMCPFile validates that the file is a valid MCP-related file
func isValidMCPFile(filename string) bool {
	validExtensions := []string{".py", ".txt", ".md", ".json"}
	for _, ext := range validExtensions {
		if strings.HasSuffix(filename, ext) {
			return true
		}
	}
	return false
}

// generateID generates a unique ID for MCP servers
func generateID() string {
	return time.Now().Format("20060102150405") + fmt.Sprintf("%06d", time.Now().Nanosecond()/1000)
}
