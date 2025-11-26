package handlers

import (
	crand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/models"
)

// RegistryManagerInterface defines the interface for registry management
type RegistryManagerInterface interface {
	UploadRegistryEntries(entries []*models.MCPServer) error
	LoadFromCustomSource(sourceURL string) error
	SearchServers(query string, filters map[string]interface{}) ([]*models.MCPServer, error)
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
	Store             MCPServerStore
	RegistryManager   RegistryManagerInterface
	DeploymentHandler *DeploymentHandler
	AdapterStore      *clients.InMemoryAdapterStore
}

// NewRegistryHandler creates a new registry handler
func NewRegistryHandler(store MCPServerStore, registryManager RegistryManagerInterface, deploymentHandler *DeploymentHandler, adapterStore *clients.InMemoryAdapterStore) *RegistryHandler {
	return &RegistryHandler{
		Store:             store,
		RegistryManager:   registryManager,
		DeploymentHandler: deploymentHandler,
		AdapterStore:      adapterStore,
	}
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

// PublicList handles GET /public/registry
// @Summary Get public registry data
// @Description Retrieve filtered JSON data from MCP registries (official or docker)
// @Tags registry
// @Produce json
// @Param source query string false "Registry source: 'official' or 'docker'" Enums(official,docker)
// @Param provider query string false "Filter by provider (works for both official and docker sources)"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {string} string "Internal Server Error"
// @Router /api/v1/registry/public [get]
func (h *RegistryHandler) PublicList(c *gin.Context) {
	source := c.Query("source")
	if source == "" {
		source = "official" // Default to official registry
	}

	provider := c.Query("provider")

	log.Printf("PublicList called with source=%s, provider=%s", source, provider)

	switch source {
	case "docker":
		log.Printf("Routing to Docker registry fetch")
		h.fetchDockerRegistry(c, provider)
	case "official":
		log.Printf("Routing to official registry fetch")
		h.fetchOfficialRegistry(c, provider)
	default:
		log.Printf("Invalid source parameter: %s", source)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid source. Must be 'official' or 'docker'"})
	}
}

// fetchOfficialRegistry fetches from the official MCP registry
func (h *RegistryHandler) fetchOfficialRegistry(c *gin.Context, provider string) {
	log.Printf("Fetching official registry data from: https://registry.modelcontextprotocol.io/v0.1/servers")

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Build URL with provider filter if specified
	url := "https://registry.modelcontextprotocol.io/v0.1/servers?limit=100"
	if provider != "" {
		url += "&provider=" + provider
	}

	// Fetch data from the public registry
	resp, err := client.Get(url)
	if err != nil {
		log.Printf("Error fetching official registry: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch official registry: %v", err)})
		return
	}
	defer resp.Body.Close()

	log.Printf("Official registry response status: %d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		log.Printf("Official registry returned non-200 status: %d", resp.StatusCode)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Official registry unavailable (status: %d)", resp.StatusCode)})
		return
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("Error decoding official registry response: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse registry response"})
		return
	}

	// Filter servers to only include active latest versions
	if servers, ok := result["servers"].([]interface{}); ok {
		log.Printf("Received %d servers from official registry", len(servers))
		var filteredServers []interface{}

		for _, serverEntry := range servers {
			if serverMap, ok := serverEntry.(map[string]interface{}); ok {
				if meta, ok := serverMap["_meta"].(map[string]interface{}); ok {
					if official, ok := meta["io.modelcontextprotocol.registry/official"].(map[string]interface{}); ok {
						status, hasStatus := official["status"].(string)
						isLatest, hasIsLatest := official["isLatest"].(bool)

						if hasStatus && hasIsLatest && status == "active" && isLatest {
							// Add validation_status to the server entry
							serverMap["validation_status"] = "new"
							filteredServers = append(filteredServers, serverMap)
						}
					}
				}
			}
		}

		log.Printf("Filtered to %d active latest servers", len(filteredServers))
		result["servers"] = filteredServers

		// Store servers in local registry for browsing
		h.storeFetchedServers(filteredServers, "official")
	}

	c.JSON(http.StatusOK, result)
}

// storeFetchedServers converts and stores fetched servers in the local registry
func (h *RegistryHandler) storeFetchedServers(servers []interface{}, source string) {
	for _, serverData := range servers {
		if serverMap, ok := serverData.(map[string]interface{}); ok {
			server := h.convertToMCPServer(serverMap, source)
			if server != nil {
				// Check if server already exists
				if existing, _ := h.Store.GetMCPServer(server.ID); existing == nil {
					if err := h.Store.CreateMCPServer(server); err != nil {
						log.Printf("Error storing server %s: %v", server.ID, err)
					} else {
						log.Printf("Stored server %s in local registry", server.ID)
					}
				} else {
					log.Printf("Server %s already exists in local registry", server.ID)
				}
			}
		}
	}
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
		repo := &models.Repository{}
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

	// Extract configuration template for Docker images
	if source == "docker-mcp" {
		// Find the Docker image from packages
		for _, pkg := range server.Packages {
			if pkg.RegistryType == "oci" && strings.HasPrefix(pkg.Identifier, "mcp/") {
				configTemplate, err := h.extractDockerConfig(pkg.Identifier)
				if err != nil {
					log.Printf("Failed to extract config for %s: %v", pkg.Identifier, err)
				} else {
					server.ConfigTemplate = configTemplate
				}
				break
			}
		}
	}

	return server
}

// fetchDockerRegistry fetches from Docker Hub MCP namespace
func (h *RegistryHandler) fetchDockerRegistry(c *gin.Context, provider string) {
	log.Printf("Fetching Docker registry data from: https://hub.docker.com/v2/repositories/mcp/")

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	var allServers []map[string]interface{}

	// Docker Hub API pagination
	url := "https://hub.docker.com/v2/repositories/mcp/?page_size=100"

	for url != "" {
		resp, err := client.Get(url)
		if err != nil {
			log.Printf("Error fetching Docker registry: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch Docker registry: %v", err)})
			return
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			log.Printf("Docker registry returned non-200 status: %d", resp.StatusCode)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Docker registry unavailable (status: %d)", resp.StatusCode)})
			return
		}

		var dockerResponse struct {
			Count    int    `json:"count"`
			Next     string `json:"next"`
			Previous string `json:"previous"`
			Results  []struct {
				Name        string `json:"name"`
				Namespace   string `json:"namespace"`
				Description string `json:"description"`
				StarCount   int    `json:"star_count"`
				PullCount   int    `json:"pull_count"`
				LastUpdated string `json:"last_updated"`
				Categories  []struct {
					Name string `json:"name"`
					Slug string `json:"slug"`
				} `json:"categories"`
			} `json:"results"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&dockerResponse); err != nil {
			resp.Body.Close()
			log.Printf("Error decoding Docker registry response: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse Docker registry response"})
			return
		}

		resp.Body.Close()

		// Convert Docker repositories to MCP server format
		for _, repo := range dockerResponse.Results {
			// Apply provider filtering if specified
			if provider != "" && !h.matchesProvider(repo, provider) {
				continue
			}

			server := map[string]interface{}{
				"id":          fmt.Sprintf("docker-mcp-%s", repo.Name),
				"name":        fmt.Sprintf("mcp/%s", repo.Name),
				"description": repo.Description,
				"repository": map[string]interface{}{
					"url":    fmt.Sprintf("https://hub.docker.com/r/mcp/%s", repo.Name),
					"source": "dockerhub",
				},
				"packages": []map[string]interface{}{
					{
						"registryType": "oci",
						"identifier":   fmt.Sprintf("mcp/%s", repo.Name),
						"transport": map[string]interface{}{
							"type": "stdio",
						},
					},
				},
				"validation_status": "new",
				"_meta": map[string]interface{}{
					"source":       "docker-mcp",
					"provider":     h.inferProvider(repo),
					"stars":        repo.StarCount,
					"pulls":        repo.PullCount,
					"last_updated": repo.LastUpdated,
					"icon_url":     fmt.Sprintf("https://api.scout.docker.com/v1/policy/insights/org-image-score/badge/mcp/%s", repo.Name),
				},
			}

			// Add basic tool info
			if repo.Description != "" {
				server["tools"] = []map[string]interface{}{
					{
						"name":        "execute",
						"description": repo.Description,
						"input_schema": map[string]interface{}{
							"type":       "object",
							"properties": map[string]interface{}{},
						},
					},
				}
			}

			allServers = append(allServers, server)
		}

		// Get next page URL
		url = dockerResponse.Next
	}

	log.Printf("Fetched %d servers from Docker registry", len(allServers))

	// Store servers in local registry for browsing
	var serversInterface []interface{}
	for _, s := range allServers {
		serversInterface = append(serversInterface, s)
	}
	h.storeFetchedServers(serversInterface, "docker-mcp")

	result := map[string]interface{}{
		"servers": allServers,
		"source":  "docker-mcp",
		"count":   len(allServers),
	}

	c.JSON(http.StatusOK, result)
}

// matchesProvider checks if a Docker repository matches the specified provider
func (h *RegistryHandler) matchesProvider(repo struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Description string `json:"description"`
	StarCount   int    `json:"star_count"`
	PullCount   int    `json:"pull_count"`
	LastUpdated string `json:"last_updated"`
	Categories  []struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	} `json:"categories"`
}, provider string) bool {
	inferredProvider := h.inferProvider(repo)
	return strings.EqualFold(inferredProvider, provider)
}

// inferProvider attempts to infer the provider from Docker repository metadata
func (h *RegistryHandler) inferProvider(repo struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Description string `json:"description"`
	StarCount   int    `json:"star_count"`
	PullCount   int    `json:"pull_count"`
	LastUpdated string `json:"last_updated"`
	Categories  []struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	} `json:"categories"`
}) string {
	// Check for official Anthropic servers
	officialServers := map[string]bool{
		"everything":         true,
		"filesystem":         true,
		"memory":             true,
		"sequentialthinking": true,
	}

	if officialServers[repo.Name] {
		return "anthropic"
	}

	// Check categories for provider hints
	for _, category := range repo.Categories {
		if strings.Contains(strings.ToLower(category.Name), "official") {
			return "official"
		}
	}

	// Check description for provider hints
	desc := strings.ToLower(repo.Description)
	if strings.Contains(desc, "official") || strings.Contains(desc, "anthropic") {
		return "anthropic"
	}

	// Default to community for most servers
	return "community"
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

// BrowseRegistry handles GET /registry/browse
// @Summary Browse registry servers with search and filters
// @Description Search and filter MCP servers from all configured sources
// @Tags registry
// @Produce json
// @Param q query string false "Search query"
// @Param transport query string false "Filter by transport type (stdio, sse, websocket)"
// @Param registryType query string false "Filter by registry type (oci, npm)"
// @Param validationStatus query string false "Filter by validation status"
// @Param source query string false "Filter by source (official, docker-mcp, virtualmcp)"
// @Success 200 {array} models.MCPServer
// @Router /api/v1/registry/browse [get]
func (h *RegistryHandler) BrowseRegistry(c *gin.Context) {
	query := c.Query("q")

	filters := make(map[string]interface{})
	if transport := c.Query("transport"); transport != "" {
		filters["transport"] = transport
	}
	if registryType := c.Query("registryType"); registryType != "" {
		filters["registryType"] = registryType
	}
	if validationStatus := c.Query("validationStatus"); validationStatus != "" {
		filters["validationStatus"] = validationStatus
	}
	if source := c.Query("source"); source != "" {
		filters["source"] = source
	}

	servers, err := h.RegistryManager.SearchServers(query, filters)
	if err != nil {
		log.Printf("Error searching registry: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to search registry"})
		return
	}

	c.JSON(http.StatusOK, servers)
}

// SyncOfficialRegistry handles POST /registry/sync/official
// @Summary Sync from official MCP registry
// @Description Manually trigger synchronization with the official MCP registry
// @Tags registry
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/registry/sync/official [post]
func (h *RegistryHandler) SyncOfficialRegistry(c *gin.Context) {
	// For now, return a placeholder response
	// In full implementation, this would trigger the RegistryManager.SyncOfficialRegistry()
	response := map[string]interface{}{
		"message": "Official registry sync not yet implemented",
		"status":  "pending",
	}

	c.JSON(http.StatusOK, response)
}

// CreateAdapterFromRegistry handles POST /registry/{id}/create-adapter
// @Summary Create an adapter from an MCP registry entry
// @Description Creates an adapter from an MCP server in the registry, specifically for virtualMCP servers
// @Tags registry, adapters
// @Accept json
// @Produce json
// @Param id path string true "MCP Server ID"
// @Param request body CreateAdapterFromRegistryRequest true "Adapter creation configuration"
// @Success 201 {object} CreateAdapterFromRegistryResponse
// @Failure 400 {string} string "Bad Request"
// @Failure 404 {string} string "Server not found"
// @Router /api/v1/registry/{id}/create-adapter [post]
func (h *RegistryHandler) CreateAdapterFromRegistry(c *gin.Context) {
	serverID := c.Param("id")
	if serverID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Server ID is required"})
		return
	}

	// Get the MCP server from registry
	server, err := h.Store.GetMCPServer(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "MCP server not found"})
		return
	}

	// Check if this is a virtualMCP server
	if server.Meta == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Server is not a virtualMCP server"})
		return
	}

	source, ok := server.Meta["source"].(string)
	if !ok || source != "virtualmcp" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Server is not a virtualMCP server"})
		return
	}

	// Validate VirtualMCP tools configuration
	if err := h.validateVirtualMCPTools(server.Tools); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid tool configuration: %v", err)})
		return
	}

	// Parse request body for additional configuration
	var req CreateAdapterFromRegistryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = CreateAdapterFromRegistryRequest{} // Use defaults
	}

	// Generate authentication token
	tokenBytes := make([]byte, 32)
	if _, err := crand.Read(tokenBytes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}
	tokenStr := base64.URLEncoding.EncodeToString(tokenBytes)
	expiresAt := time.Now().Add(24 * time.Hour)

	// For VirtualMCP adapters, we deploy the virtualmcp-server.ts as a streamable HTTP server
	// and create a StreamableHttp adapter that connects to it

	// Generate adapter name
	adapterName := fmt.Sprintf("virtualmcp-%s", strings.ReplaceAll(server.ID, "/", "-"))

	// Prepare tools configuration for the server - convert to template format
	templateTools := convertToolsForTemplate(server.Tools)
	toolsJSON, err := json.Marshal(templateTools)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal tools configuration"})
		return
	}

	// Update the server's ConfigTemplate for deployment as HTTP server
	server.ConfigTemplate = &models.MCPConfigTemplate{
		Command: "npx",
		Args:    []string{"tsx", "templates/virtualmcp-server.ts", "--transport", "http"},
		Env: map[string]string{
			"SERVER_NAME":  adapterName,
			"PORT":         "8080", // Default port for the HTTP server
			"BEARER_TOKEN": tokenStr,
			"TOOLS_CONFIG": string(toolsJSON), // Use TOOLS_CONFIG instead of VIRTUAL_MCP_CONFIG
		},
		Transport: "http",
		Image:     "ghcr.io/alessandro-festa/suse-ai-up:latest",
	}

	// Add environment variables from request
	if req.EnvironmentVariables != nil {
		for k, v := range req.EnvironmentVariables {
			server.ConfigTemplate.Env[k] = v
		}
	}

	// Update the server in the registry
	if err := h.Store.UpdateMCPServer(server.ID, server); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update server configuration"})
		return
	}

	// Deploy the server
	if err := h.DeploymentHandler.DeployMCPDirect(server.ID, req.EnvironmentVariables, req.ReplicaCount); err != nil {
		log.Printf("Failed to deploy VirtualMCP server: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to deploy server: %v", err)})
		return
	}

	// Create StreamableHttp adapter that connects to the deployed server
	adapterData := &models.AdapterData{
		Name:           adapterName,
		Protocol:       models.ServerProtocolMCP,
		ConnectionType: models.ConnectionTypeStreamableHttp,
		ReplicaCount:   req.ReplicaCount,
		Description:    fmt.Sprintf("VirtualMCP adapter for %s", server.Name),
		RemoteUrl:      fmt.Sprintf("http://mcp-%s", strings.ReplaceAll(server.ID, "/", "-")), // Service URL for deployed server
		Authentication: &models.AdapterAuthConfig{
			Required: true,
			Type:     "bearer",
			BearerToken: &models.BearerTokenConfig{
				Token:     tokenStr,
				Dynamic:   false,
				ExpiresAt: expiresAt,
			},
		},
	}

	// Create adapter resource
	adapter := &models.AdapterResource{}
	adapter.Create(*adapterData, "system", time.Now())

	// Store the adapter
	if err := h.AdapterStore.Create(adapter); err != nil {
		log.Printf("Failed to store adapter: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to store adapter: %v", err)})
		return
	}

	tokenInfo := &AuthTokenInfo{
		Token:     tokenStr,
		TokenType: "Bearer",
		ExpiresAt: expiresAt,
	}

	response := CreateAdapterFromRegistryResponse{
		Message:   "VirtualMCP adapter created and deployed successfully",
		Adapter:   adapter,
		TokenInfo: tokenInfo,
		Note:      "VirtualMCP server is now running and ready to use",
	}

	c.JSON(http.StatusCreated, response)
}

// CreateAdapterFromRegistryRequest represents the request for creating an adapter from registry
type CreateAdapterFromRegistryRequest struct {
	ReplicaCount         int               `json:"replicaCount,omitempty"`
	EnvironmentVariables map[string]string `json:"environmentVariables,omitempty"`
}

// CreateAdapterFromRegistryResponse represents the response for adapter creation
type CreateAdapterFromRegistryResponse struct {
	Message   string                  `json:"message"`
	Adapter   *models.AdapterResource `json:"adapter"`
	TokenInfo *AuthTokenInfo          `json:"tokenInfo,omitempty"`
	Note      string                  `json:"note"`
}

// AuthTokenInfo represents authentication token information
type AuthTokenInfo struct {
	Token     string    `json:"token"`
	TokenType string    `json:"tokenType"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// validateVirtualMCPTools validates that VirtualMCP tools have proper configuration
func (h *RegistryHandler) validateVirtualMCPTools(tools []models.MCPTool) error {
	for _, tool := range tools {
		if tool.SourceType == "" {
			continue // Skip validation for non-VirtualMCP tools
		}

		switch tool.SourceType {
		case "api":
			if tool.Config == nil {
				return fmt.Errorf("tool %s: config is required for API tools", tool.Name)
			}
			if _, ok := tool.Config["api_url"]; !ok {
				return fmt.Errorf("tool %s: api_url is required for API tools", tool.Name)
			}
		case "database":
			if tool.Config == nil {
				return fmt.Errorf("tool %s: config is required for database tools", tool.Name)
			}
			if _, ok := tool.Config["db_type"]; !ok {
				return fmt.Errorf("tool %s: db_type is required for database tools", tool.Name)
			}
			if _, ok := tool.Config["db_connection"]; !ok {
				return fmt.Errorf("tool %s: db_connection is required for database tools", tool.Name)
			}
			if _, ok := tool.Config["db_query"]; !ok {
				return fmt.Errorf("tool %s: db_query is required for database tools", tool.Name)
			}
		case "graphql":
			if tool.Config == nil {
				return fmt.Errorf("tool %s: config is required for GraphQL tools", tool.Name)
			}
			if _, ok := tool.Config["graphql_url"]; !ok {
				return fmt.Errorf("tool %s: graphql_url is required for GraphQL tools", tool.Name)
			}
			if _, ok := tool.Config["graphql_query"]; !ok {
				return fmt.Errorf("tool %s: graphql_query is required for GraphQL tools", tool.Name)
			}
		default:
			return fmt.Errorf("tool %s: unsupported source type: %s", tool.Name, tool.SourceType)
		}
	}
	return nil
}

// createAdapterDataFromMCPServer creates adapter configuration from MCP server
func (h *RegistryHandler) createAdapterDataFromMCPServer(server *models.MCPServer, req CreateAdapterFromRegistryRequest) *models.AdapterData {
	// Generate adapter name
	adapterName := fmt.Sprintf("virtualmcp-%s", strings.ReplaceAll(server.ID, "/", "-"))

	// Create adapter data with streamable HTTP transport for virtualMCP
	adapterData := &models.AdapterData{
		Name:                 adapterName,
		ImageName:            "ghcr.io/alessandro-festa/suse-ai-up", // Use main suse-ai-up image
		ImageVersion:         "latest",
		Protocol:             models.ServerProtocolMCP,
		ConnectionType:       models.ConnectionTypeStreamableHttp,
		Command:              "tsx",
		Args:                 []string{"templates/virtualmcp-server.ts", "--transport", "http"},
		EnvironmentVariables: make(map[string]string),
		ReplicaCount:         req.ReplicaCount,
		Description:          fmt.Sprintf("VirtualMCP adapter for %s", server.Name),
		UseWorkloadIdentity:  false,
		RemoteUrl:            fmt.Sprintf("http://%s:3000", adapterName), // Service URL for deployed adapter
	}

	if adapterData.ReplicaCount <= 0 {
		adapterData.ReplicaCount = 1
	}

	// Set default environment variables
	adapterData.EnvironmentVariables["SERVER_NAME"] = server.Name
	adapterData.EnvironmentVariables["PORT"] = "3000"
	adapterData.EnvironmentVariables["MCP_PROXY_URL"] = fmt.Sprintf("http://%s:3000/mcp", adapterName)

	// Set environment variables from request
	if req.EnvironmentVariables != nil {
		for k, v := range req.EnvironmentVariables {
			adapterData.EnvironmentVariables[k] = v
		}
	}

	// Add tools configuration
	if len(server.Tools) > 0 {
		toolsJSON, err := json.Marshal(server.Tools)
		if err == nil {
			adapterData.EnvironmentVariables["TOOLS_CONFIG"] = string(toolsJSON)
		}
	}

	return adapterData
}

// generateAuthForVirtualMCPAdapter generates authentication for virtualMCP adapter
func (h *RegistryHandler) generateAuthForVirtualMCPAdapter(adapterData *models.AdapterData) (*AuthTokenInfo, error) {
	// Generate a secure token
	token := make([]byte, 32)
	if _, err := crand.Read(token); err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	tokenStr := base64.URLEncoding.EncodeToString(token)

	expiresAt := time.Now().Add(24 * time.Hour)

	// Configure authentication
	adapterData.Authentication = &models.AdapterAuthConfig{
		Required: true,
		Type:     "bearer",
		BearerToken: &models.BearerTokenConfig{
			Token:     tokenStr,
			Dynamic:   false,
			ExpiresAt: expiresAt,
		},
	}

	return &AuthTokenInfo{
		Token:     tokenStr,
		TokenType: "Bearer",
		ExpiresAt: expiresAt,
	}, nil
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

// extractDockerConfig extracts MCP server configuration from Docker Hub page
func (h *RegistryHandler) extractDockerConfig(dockerImage string) (*models.MCPConfigTemplate, error) {
	// Extract namespace and repo from image name (e.g., "mcp/brave-search" -> "mcp", "brave-search")
	parts := strings.Split(dockerImage, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid docker image format: %s", dockerImage)
	}
	namespace, repo := parts[0], parts[1]

	// Construct Docker Hub URL
	url := fmt.Sprintf("https://hub.docker.com/r/%s/%s", namespace, repo)

	// Fetch the page
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Docker Hub page: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Docker Hub returned status %d", resp.StatusCode)
	}

	// Read the page content
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	content := string(body)

	// Extract configuration from embedded JSON data
	config := &models.MCPConfigTemplate{
		Command:   "docker",
		Args:      []string{"run", "--rm", "-i", dockerImage},
		Env:       make(map[string]string),
		Transport: "stdio", // default
		Image:     dockerImage,
	}

	// Extract environment variables using multiple patterns for robustness
	envPatterns := []*regexp.Regexp{
		// Pattern for quoted env vars like "BRAVE_API_KEY": "YOUR_API_KEY_HERE"
		regexp.MustCompile(`"([A-Z_][A-Z0-9_]*_KEY)"\s*:\s*"([^"]*)"`),
		// Pattern for env vars in configuration objects
		regexp.MustCompile(`([A-Z_][A-Z0-9_]*_KEY)\s*:\s*["']([^"']*)["']`),
		// Pattern for env vars in comments or documentation
		regexp.MustCompile(`([A-Z_][A-Z0-9_]*_KEY)\s*=\s*([^,\s\n]+)`),
	}

	foundEnvVars := make(map[string]bool)
	for _, pattern := range envPatterns {
		matches := pattern.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) >= 3 {
				envKey := match[1]
				envValue := match[2]
				// Only add if it's a placeholder or empty value
				if envValue == "" || envValue == "YOUR_API_KEY_HERE" || strings.Contains(strings.ToUpper(envValue), "YOUR_") || strings.Contains(envValue, "API_KEY") {
					if !foundEnvVars[envKey] {
						config.Env[envKey] = ""
						foundEnvVars[envKey] = true
					}
				}
			}
		}
	}

	// Extract from secrets array if present
	secretsPattern := regexp.MustCompile(`"secrets"\s*:\s*\[([^\]]+)\]`)
	secretsMatch := secretsPattern.FindStringSubmatch(content)
	if secretsMatch != nil {
		secretsContent := secretsMatch[1]
		secretPattern := regexp.MustCompile(`"([^"]+)"`)
		secretMatches := secretPattern.FindAllStringSubmatch(secretsContent, -1)

		for _, secretMatch := range secretMatches {
			if len(secretMatch) >= 2 {
				secretName := secretMatch[1]
				// Convert to env var format
				envKey := strings.ToUpper(strings.ReplaceAll(secretName, ".", "_"))
				envKey = strings.ReplaceAll(envKey, "-", "_")
				if strings.HasSuffix(envKey, "_KEY") || strings.Contains(envKey, "API") || strings.Contains(envKey, "TOKEN") {
					if !foundEnvVars[envKey] {
						config.Env[envKey] = ""
						foundEnvVars[envKey] = true
					}
				}
			}
		}
	}

	// Determine transport type from the content
	transportPatterns := map[string]string{
		`"type"\s*:\s*"streamable-http"`: "http",
		`"type"\s*:\s*"sse"`:             "sse",
		`"transport"\s*:\s*"http"`:       "http",
		`"transport"\s*:\s*"sse"`:        "sse",
	}

	for pattern, transportType := range transportPatterns {
		if matched, _ := regexp.MatchString(pattern, content); matched {
			config.Transport = transportType
			break
		}
	}

	return config, nil
}

// generateID generates a unique ID for MCP servers
func generateID() string {
	return time.Now().Format("20060102150405") + fmt.Sprintf("%06d", time.Now().Nanosecond()/1000)
}

// TemplateTool represents the tool format expected by the VirtualMCP template
type TemplateTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	SourceType  string                 `json:"source_type"`
	InputSchema map[string]interface{} `json:"input_schema"`
	Config      map[string]interface{} `json:"config"`
}

// convertToolsForTemplate converts MCPTool format to VirtualMCPTool format expected by the template
func convertToolsForTemplate(tools []models.MCPTool) []TemplateTool {
	templateTools := make([]TemplateTool, len(tools))
	for i, tool := range tools {
		templateTools[i] = TemplateTool{
			Name:        tool.Name,
			Description: tool.Description,
			SourceType:  tool.SourceType,
			InputSchema: tool.InputSchema,
			Config:      tool.Config,
		}
		// Set defaults if missing
		if templateTools[i].SourceType == "" {
			templateTools[i].SourceType = "api"
		}
		if templateTools[i].Config == nil {
			templateTools[i].Config = make(map[string]interface{})
		}
	}
	return templateTools
}
