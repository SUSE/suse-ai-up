package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
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
	Store           MCPServerStore
	RegistryManager RegistryManagerInterface
}

// NewRegistryHandler creates a new registry handler
func NewRegistryHandler(store MCPServerStore, registryManager RegistryManagerInterface) *RegistryHandler {
	return &RegistryHandler{
		Store:           store,
		RegistryManager: registryManager,
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
// @Router /registry/{id} [get]
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
// @Router /registry/{id} [put]
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
// @Router /registry/{id} [delete]
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
// @Description Retrieve filtered JSON data from the official Model Context Protocol registry (only active latest versions)
// @Tags registry
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {string} string "Internal Server Error"
// @Router /public/registry [get]
func (h *RegistryHandler) PublicList(c *gin.Context) {
	log.Printf("Fetching public registry data from: https://registry.modelcontextprotocol.io/v0/servers")

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Fetch data from the public registry
	resp, err := client.Get("https://registry.modelcontextprotocol.io/v0/servers?limit=100")
	if err != nil {
		log.Printf("Error fetching public registry: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch public registry: %v", err)})
		return
	}
	defer resp.Body.Close()

	log.Printf("Public registry response status: %d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		log.Printf("Public registry returned non-200 status: %d", resp.StatusCode)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Public registry unavailable (status: %d)", resp.StatusCode)})
		return
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("Error decoding public registry response: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse registry response"})
		return
	}

	// Filter servers to only include active latest versions
	if servers, ok := result["servers"].([]interface{}); ok {
		log.Printf("Received %d servers from registry", len(servers))
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
	}

	c.JSON(http.StatusOK, result)
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
// @Router /registry/upload [post]
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
// @Router /registry/upload/bulk [post]
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
// @Success 200 {array} models.MCPServer
// @Router /registry/browse [get]
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
// @Router /registry/sync/official [post]
func (h *RegistryHandler) SyncOfficialRegistry(c *gin.Context) {
	// For now, return a placeholder response
	// In full implementation, this would trigger the RegistryManager.SyncOfficialRegistry()
	response := map[string]interface{}{
		"message": "Official registry sync not yet implemented",
		"status":  "pending",
	}

	c.JSON(http.StatusOK, response)
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
// @Router /registry/upload/local-mcp [post]
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
