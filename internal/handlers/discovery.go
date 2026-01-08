package handlers

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/scanner"
)

type DiscoveryHandler struct {
	scanManager *scanner.ScanManager
	store       scanner.DiscoveryStore
}

func NewDiscoveryHandler(scanManager *scanner.ScanManager, store scanner.DiscoveryStore) *DiscoveryHandler {
	return &DiscoveryHandler{
		scanManager: scanManager,
		store:       store,
	}
}

// ScanForMCPServers performs network scanning to discover MCP servers
// @Summary Start network scan for MCP servers
// @Description Initiates a network scan to discover MCP servers and returns a job ID
// @Tags discovery
// @Accept json
// @Produce json
// @Param config body models.ScanConfig true "Scan configuration"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/v1/discovery/scan [post]
func (h *DiscoveryHandler) ScanForMCPServers(c *gin.Context) {
	// Parse scan configuration from request body
	var scanConfig models.ScanConfig
	if err := c.ShouldBindJSON(&scanConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid scan configuration: " + err.Error()})
		return
	}

	// Log the received config for debugging
	log.Printf("DEBUG: Received scan config: %+v", scanConfig)

	// Set defaults for missing fields
	if scanConfig.Timeout == "" {
		scanConfig.Timeout = "30s"
	}
	if scanConfig.MaxConcurrent == 0 {
		scanConfig.MaxConcurrent = 10
	}
	if scanConfig.ExcludeProxy == nil {
		excludeProxy := true
		scanConfig.ExcludeProxy = &excludeProxy
	}

	// Validate scan ranges if provided
	if len(scanConfig.ScanRanges) > 0 {
		for _, scanRange := range scanConfig.ScanRanges {
			if !h.isValidScanRange(scanRange) {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid scan range format: %s", scanRange)})
				return
			}
		}
	} else {
		// Set default scan ranges if none provided
		defaultRanges, err := h.getDefaultScanRanges()
		if err != nil {
			log.Printf("DEBUG: Failed to get default scan ranges: %v", err)
			// Fallback to localhost
			scanConfig.ScanRanges = []string{"127.0.0.1/32"}
		} else {
			scanConfig.ScanRanges = defaultRanges
			log.Printf("DEBUG: Using default scan ranges: %v", defaultRanges)
		}
	}

	// Set default ports if none provided
	if len(scanConfig.Ports) == 0 {
		scanConfig.Ports = []string{"8000", "8001", "8002", "8003", "8004", "8080", "8888"}
		log.Printf("DEBUG: Using default ports: %v", scanConfig.Ports)
	}

	// Start the scan job
	job, err := h.scanManager.StartScan(scanConfig)
	if err != nil {
		log.Printf("ERROR: Failed to start scan: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start scan: " + err.Error()})
		return
	}

	log.Printf("DEBUG: Started scan job %s", job.ID)

	c.JSON(http.StatusOK, gin.H{
		"jobId":   job.ID,
		"status":  job.Status,
		"message": job.Message,
	})
}

// ListDiscoveredServers returns all discovered MCP servers
// @Summary List discovered servers (DEPRECATED)
// @Description This endpoint is deprecated due to data access issues. Use GET /api/v1/discovery/results instead.
// @Tags discovery
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/discovery/servers [get]
// @Deprecated
func (h *DiscoveryHandler) ListDiscoveredServers(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{
		"error":       "This endpoint is deprecated and disabled",
		"message":     "Server listing functionality has been moved to GET /api/v1/discovery/results",
		"deprecated":  true,
		"alternative": "GET /api/v1/discovery/results",
	})
}

// @Summary Get discovered server by ID (DEPRECATED)
// @Description This endpoint is deprecated due to data access issues. Use GET /api/v1/discovery/results/{id} instead.
// @Tags discovery
// @Produce json
// @Param id path string true "Server ID"
// @Success 200 {object} models.DiscoveredServer
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/discovery/servers/{id} [get]
// @Deprecated
func (h *DiscoveryHandler) GetDeprecatedServer(c *gin.Context) {
	serverID := c.Param("id")
	c.JSON(http.StatusGone, gin.H{
		"error":       "This endpoint is deprecated and disabled",
		"server_id":   serverID,
		"message":     "Server retrieval functionality has been moved to GET /api/v1/discovery/results/{id}",
		"deprecated":  true,
		"alternative": "GET /api/v1/discovery/results/{id}",
	})
}

// @Summary Get all scan results
// @Description Returns aggregated results from all completed scans
// @Tags discovery
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/discovery/results [get]
func (h *DiscoveryHandler) GetAllScanResults(c *gin.Context) {
	log.Printf("DEBUG: GetAllScanResults called")

	// Get all scan jobs
	jobs := h.scanManager.ListJobs()

	// Aggregate results from all completed scans
	allResults := []models.DiscoveredServer{}
	scanSummaries := []gin.H{}

	for _, job := range jobs {
		if job.Status == scanner.ScanStatusCompleted && len(job.Results) > 0 {
			// Add all results from this completed scan
			allResults = append(allResults, job.Results...)

			// Create summary for this scan
			scanSummary := gin.H{
				"scan_id":       job.ID,
				"start_time":    job.StartTime,
				"end_time":      job.EndTime,
				"servers_found": len(job.Results),
				"config":        job.Config,
			}
			scanSummaries = append(scanSummaries, scanSummary)
		}
	}

	// Remove duplicates based on server ID (keep the most recent)
	uniqueResults := h.deduplicateServers(allResults)

	response := gin.H{
		"total_scans":    len(scanSummaries),
		"total_servers":  len(uniqueResults),
		"scan_summaries": scanSummaries,
		"servers":        h.safeServerList(uniqueResults),
	}

	log.Printf("DEBUG: Returning %d unique servers from %d scans", len(uniqueResults), len(scanSummaries))
	c.JSON(http.StatusOK, response)
}

// deduplicateServers removes duplicate servers based on ID, keeping the most recent
func (h *DiscoveryHandler) deduplicateServers(servers []models.DiscoveredServer) []models.DiscoveredServer {
	seen := make(map[string]models.DiscoveredServer)
	for _, server := range servers {
		if existing, exists := seen[server.ID]; !exists || server.LastSeen.After(existing.LastSeen) {
			seen[server.ID] = server
		}
	}

	result := make([]models.DiscoveredServer, 0, len(seen))
	for _, server := range seen {
		result = append(result, server)
	}

	return result
}

// safeServerList converts server list to safe format for JSON response
func (h *DiscoveryHandler) safeServerList(servers []models.DiscoveredServer) []gin.H {
	result := make([]gin.H, 0, len(servers))

	for _, server := range servers {
		safeServer := gin.H{
			"id":                  server.ID,
			"name":                server.Name,
			"address":             server.Address,
			"protocol":            string(server.Protocol),
			"connection":          string(server.Connection),
			"status":              server.Status,
			"last_seen":           server.LastSeen.Format(time.RFC3339),
			"vulnerability_score": server.VulnerabilityScore,
			"server_version":      server.ServerVersion,
			"protocol_version":    server.ProtocolVersion,
		}

		// Safely add metadata
		if server.Metadata != nil && len(server.Metadata) > 0 {
			safeServer["metadata"] = server.Metadata
		}

		// Safely add capabilities
		if server.Capabilities != nil {
			capabilities := gin.H{}
			capabilities["tools"] = server.Capabilities.Tools
			capabilities["resources"] = server.Capabilities.Resources
			capabilities["prompts"] = server.Capabilities.Prompts
			capabilities["logging"] = server.Capabilities.Logging
			capabilities["completions"] = server.Capabilities.Completions
			capabilities["experimental"] = server.Capabilities.Experimental
			safeServer["capabilities"] = capabilities
		}

		// Add tool/resource/prompt counts without accessing the arrays
		if server.Metadata != nil {
			if toolsCount := extractCountFromMetadata(server.Metadata, "tools_count"); toolsCount > 0 {
				safeServer["tools_count"] = toolsCount
			}
			if resourcesCount := extractCountFromMetadata(server.Metadata, "resources_count"); resourcesCount > 0 {
				safeServer["resources_count"] = resourcesCount
			}
			if promptsCount := extractCountFromMetadata(server.Metadata, "prompts_count"); promptsCount > 0 {
				safeServer["prompts_count"] = promptsCount
			}
		}

		result = append(result, safeServer)
	}

	return result
}

// extractCountFromMetadata safely extracts count values from metadata
func extractCountFromMetadata(metadata map[string]string, key string) int {
	if metadata == nil {
		return 0
	}
	if value, exists := metadata[key]; exists {
		if count, err := strconv.Atoi(value); err == nil {
			return count
		}
	}
	return 0
}

// GetDiscoveredServer returns a specific discovered MCP server by ID from scan results
// @Summary Get discovered server
// @Description Returns a specific discovered MCP server by ID from aggregated scan results
// @Tags discovery
// @Produce json
// @Param id path string true "Server ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/discovery/results/{id} [get]
// @Summary Get server by ID from aggregated scan results
// @Description Returns a specific discovered MCP server by ID from aggregated scan results
// @Tags discovery
// @Produce json
// @Param id path string true "Server ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/discovery/results/{id} [get]
func (h *DiscoveryHandler) GetServerFromResults(c *gin.Context) {
	serverID := c.Param("id")
	if serverID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Server ID is required"})
		return
	}

	// Search through all completed scan results for the server
	jobs := h.scanManager.ListJobs()
	for _, job := range jobs {
		if job.Status == scanner.ScanStatusCompleted {
			for _, server := range job.Results {
				if server.ID == serverID {
					// Found the server, return it in safe format
					safeServers := h.safeServerList([]models.DiscoveredServer{server})
					if len(safeServers) > 0 {
						c.JSON(http.StatusOK, safeServers[0])
						return
					}
				}
			}
		}
	}

	c.JSON(http.StatusNotFound, gin.H{
		"error":     "Server not found",
		"server_id": serverID,
		"note":      "Server may have been discovered in a previous scan. Check GET /api/v1/discovery/results for all available servers.",
	})
}

// ListScanJobs returns all scan jobs
// @Summary List all scan jobs
// @Description Returns all scan jobs (active and completed)
// @Tags discovery
// @Produce json
// @Success 200 {array} scanner.ScanJob
// @Router /api/v1/discovery/scan [get]
func (h *DiscoveryHandler) ListScanJobs(c *gin.Context) {
	jobs := h.scanManager.ListJobs()
	c.JSON(http.StatusOK, jobs)
}

// GetScanJob returns a specific scan job by ID
// @Summary Get scan job status
// @Description Retrieve the status and results of a network scan
// @Tags discovery
// @Produce json
// @Param jobId path string true "Scan Job ID"
// @Success 200 {object} scanner.ScanJob
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/discovery/scan/{jobId} [get]
func (h *DiscoveryHandler) GetScanJob(c *gin.Context) {
	jobID := c.Param("jobId")
	if jobID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Job ID is required"})
		return
	}

	job, err := h.scanManager.GetJob(jobID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Scan job not found"})
		return
	}

	c.JSON(http.StatusOK, job)
}

// CancelScanJob cancels a running scan job
// @Summary Cancel scan job
// @Description Cancels a running scan job
// @Tags discovery
// @Produce json
// @Param jobId path string true "Scan Job ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/discovery/scan/{jobId} [delete]
func (h *DiscoveryHandler) CancelScanJob(c *gin.Context) {
	jobID := c.Param("jobId")
	if jobID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Job ID is required"})
		return
	}

	err := h.scanManager.CancelJob(jobID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Scan job not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Scan job cancelled"})
}

// isValidScanRange validates a scan range format
func (h *DiscoveryHandler) isValidScanRange(scanRange string) bool {
	// Check if it's CIDR notation
	if strings.Contains(scanRange, "/") {
		_, _, err := net.ParseCIDR(scanRange)
		return err == nil
	}

	// Check if it's a range (e.g., "192.168.1.1-192.168.1.10")
	if strings.Contains(scanRange, "-") {
		parts := strings.Split(scanRange, "-")
		if len(parts) != 2 {
			return false
		}
		startIP := net.ParseIP(strings.TrimSpace(parts[0]))
		endIP := net.ParseIP(strings.TrimSpace(parts[1]))
		return startIP != nil && endIP != nil
	}

	// Check if it's a single IP
	return net.ParseIP(scanRange) != nil
}

// getDefaultScanRanges returns sensible default scan ranges based on local network interfaces
func (h *DiscoveryHandler) getDefaultScanRanges() ([]string, error) {
	var ranges []string

	// Add localhost
	ranges = append(ranges, "127.0.0.1/32")

	// Get network interfaces
	interfaces, err := net.Interfaces()
	if err != nil {
		return ranges, err
	}

	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP
			// Only IPv4 for now
			if ip.To4() == nil {
				continue
			}

			// Skip localhost
			if ip.IsLoopback() {
				continue
			}

			// Add /24 subnet for the IP
			ipStr := ip.String()
			// Remove last octet and add /24
			parts := net.ParseIP(ipStr).To4()
			if parts != nil {
				subnet := fmt.Sprintf("%d.%d.%d.0/24", parts[0], parts[1], parts[2])
				ranges = append(ranges, subnet)
			}
		}
	}

	return ranges, nil
}
