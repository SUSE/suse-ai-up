package handlers

import (
	"github.com/gin-gonic/gin"
	"suse-ai-up/pkg/scanner"
)

type DiscoveryHandler struct {
	networkScanner *scanner.NetworkScanner
}

func NewDiscoveryHandler(networkScanner *scanner.NetworkScanner) *DiscoveryHandler {
	return &DiscoveryHandler{
		networkScanner: networkScanner,
	}
}

// ScanForMCPServers performs network scanning to discover MCP servers
// @Summary Scan for MCP servers
// @Description Performs network scanning to discover MCP servers
// @Tags discovery
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /discovery/scan [post]
func (h *DiscoveryHandler) ScanForMCPServers(c *gin.Context) {
	// Perform network scan
	results, errors := h.networkScanner.Scan()
	c.JSON(200, gin.H{
		"discovered": results,
		"errors":     errors,
		"count":      len(results),
	})
}

// ListDiscoveredServers returns all discovered MCP servers
// @Summary List discovered servers
// @Description Returns all discovered MCP servers
// @Tags discovery
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /discovery/servers [get]
func (h *DiscoveryHandler) ListDiscoveredServers(c *gin.Context) {
	// Get all discovered servers
	servers := h.networkScanner.GetAllDiscoveredServers()
	c.JSON(200, gin.H{
		"servers": servers,
		"count":   len(servers),
	})
}

// GetDiscoveredServer returns a specific discovered MCP server by ID
// @Summary Get discovered server
// @Description Returns a specific discovered MCP server by ID
// @Tags discovery
// @Produce json
// @Param id path string true "Server ID"
// @Success 200 {object} models.DiscoveredServer
// @Failure 404 {object} map[string]interface{}
// @Router /discovery/servers/{id} [get]
func (h *DiscoveryHandler) GetDiscoveredServer(c *gin.Context) {
	// Get specific discovered server
	server := h.networkScanner.GetDiscoveredServer(c.Param("id"))
	if server == nil {
		c.JSON(404, gin.H{"error": "Server not found"})
		return
	}
	c.JSON(200, server)
}
