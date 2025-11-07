package handlers

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

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
// @Summary List discovered servers
// @Description Returns all discovered MCP servers from the persistent store
// @Tags discovery
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/discovery/servers [get]
func (h *DiscoveryHandler) ListDiscoveredServers(c *gin.Context) {
	// Get all discovered servers from persistent store
	servers, err := h.store.GetAll()
	if err != nil {
		log.Printf("ERROR: Failed to get discovered servers: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve servers"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
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
// @Router /api/v1/discovery/servers/{id} [get]
func (h *DiscoveryHandler) GetDiscoveredServer(c *gin.Context) {
	serverID := c.Param("id")
	if serverID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Server ID is required"})
		return
	}

	// Get specific discovered server from persistent store
	server, err := h.store.GetByID(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Server not found"})
		return
	}

	c.JSON(http.StatusOK, server)
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
