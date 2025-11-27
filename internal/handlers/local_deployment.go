package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// PortAllocator manages dynamic port allocation
type PortAllocator struct {
	usedPorts map[int]bool
	mutex     sync.Mutex
	minPort   int
	maxPort   int
}

// NewPortAllocator creates a new port allocator
func NewPortAllocator(minPort, maxPort int) *PortAllocator {
	return &PortAllocator{
		usedPorts: make(map[int]bool),
		minPort:   minPort,
		maxPort:   maxPort,
	}
}

// Allocate finds and reserves an available port
func (pa *PortAllocator) Allocate() (int, error) {
	pa.mutex.Lock()
	defer pa.mutex.Unlock()

	for port := pa.minPort; port <= pa.maxPort; port++ {
		if !pa.usedPorts[port] && pa.isPortAvailable(port) {
			pa.usedPorts[port] = true
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available ports in range %d-%d", pa.minPort, pa.maxPort)
}

// Release frees a previously allocated port
func (pa *PortAllocator) Release(port int) {
	pa.mutex.Lock()
	defer pa.mutex.Unlock()
	delete(pa.usedPorts, port)
}

// isPortAvailable checks if a port is available for binding
func (pa *PortAllocator) isPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// ProcessInfo tracks information about a running MCP server process
type ProcessInfo struct {
	Cmd      *exec.Cmd
	Port     int
	ServerID string
	Started  time.Time
	Pid      int
}

// LocalProcessDeploymentHandler handles MCP server deployment as local processes
type LocalProcessDeploymentHandler struct {
	runningProcesses map[string]*ProcessInfo
	portAllocator    *PortAllocator
	mutex            sync.RWMutex
	store            MCPServerStore
}

// NewLocalProcessDeploymentHandler creates a new local process deployment handler
func NewLocalProcessDeploymentHandler(store MCPServerStore, minPort, maxPort int) *LocalProcessDeploymentHandler {
	return &LocalProcessDeploymentHandler{
		runningProcesses: make(map[string]*ProcessInfo),
		portAllocator:    NewPortAllocator(minPort, maxPort),
		store:            store,
	}
}

// DeployMCPDirect deploys an MCP server as a local process
func (h *LocalProcessDeploymentHandler) DeployMCPDirect(serverID string, envVars map[string]string, replicas int) error {
	// Get server configuration
	server, err := h.store.GetMCPServer(serverID)
	if err != nil {
		return fmt.Errorf("MCP server not found: %s", serverID)
	}

	if server.ConfigTemplate == nil {
		return fmt.Errorf("Server does not have a deployment configuration")
	}

	// Allocate a port
	port, err := h.portAllocator.Allocate()
	if err != nil {
		return fmt.Errorf("failed to allocate port: %v", err)
	}

	// Prepare tools configuration
	templateTools := convertToolsForTemplate(server.Tools)
	toolsJSON, err := json.Marshal(templateTools)
	if err != nil {
		h.portAllocator.Release(port)
		return fmt.Errorf("failed to marshal tools configuration: %v", err)
	}

	// Generate authentication token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		h.portAllocator.Release(port)
		return fmt.Errorf("failed to generate token: %v", err)
	}
	tokenStr := base64.URLEncoding.EncodeToString(tokenBytes)

	// Prepare environment variables
	env := []string{
		fmt.Sprintf("PORT=%d", port),
		fmt.Sprintf("TOOLS_CONFIG=%s", string(toolsJSON)),
		fmt.Sprintf("BEARER_TOKEN=%s", tokenStr),
		fmt.Sprintf("SERVER_NAME=%s", serverID),
		"TRANSPORT_MODE=http",
	}

	// Add server-specific environment variables
	for key, value := range server.ConfigTemplate.Env {
		if key != "PORT" && key != "TOOLS_CONFIG" && key != "BEARER_TOKEN" { // Don't override our values
			env = append(env, fmt.Sprintf("%s=%s", key, value))
		}
	}

	// Add user-provided environment variables
	for key, value := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	// Start the MCP server process
	cmd := exec.Command("npx", "tsx", "templates/virtualmcp-server.ts", "--transport", "http")
	cmd.Env = env
	cmd.Dir = "." // Run from current directory

	// Capture output for logging
	// Note: In production, you might want to redirect to files or a logging system
	log.Printf("Starting MCP server process for %s on port %d", serverID, port)

	if err := cmd.Start(); err != nil {
		h.portAllocator.Release(port)
		return fmt.Errorf("failed to start MCP server process: %v", err)
	}

	// Store process information
	h.mutex.Lock()
	h.runningProcesses[serverID] = &ProcessInfo{
		Cmd:      cmd,
		Port:     port,
		ServerID: serverID,
		Started:  time.Now(),
		Pid:      cmd.Process.Pid,
	}
	h.mutex.Unlock()

	log.Printf("Successfully started MCP server %s (PID: %d) on port %d", serverID, cmd.Process.Pid, port)

	// Monitor the process in a goroutine
	go h.monitorProcess(serverID)

	return nil
}

// GetProcessInfo returns information about a running process
func (h *LocalProcessDeploymentHandler) GetProcessInfo(serverID string) (*ProcessInfo, error) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	process, exists := h.runningProcesses[serverID]
	if !exists {
		return nil, fmt.Errorf("process not found for server %s", serverID)
	}

	return process, nil
}

// StopProcess stops a running MCP server process
func (h *LocalProcessDeploymentHandler) StopProcess(serverID string) error {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	process, exists := h.runningProcesses[serverID]
	if !exists {
		return fmt.Errorf("process not found for server %s", serverID)
	}

	// Stop the process
	if err := process.Cmd.Process.Kill(); err != nil {
		log.Printf("Warning: failed to kill process for server %s: %v", serverID, err)
	}

	// Release the port
	h.portAllocator.Release(process.Port)

	// Remove from running processes
	delete(h.runningProcesses, serverID)

	log.Printf("Stopped MCP server %s", serverID)
	return nil
}

// ListRunningProcesses returns information about all running processes
func (h *LocalProcessDeploymentHandler) ListRunningProcesses() map[string]*ProcessInfo {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	result := make(map[string]*ProcessInfo)
	for k, v := range h.runningProcesses {
		result[k] = v
	}
	return result
}

// Shutdown stops all running processes
func (h *LocalProcessDeploymentHandler) Shutdown() {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	log.Printf("Shutting down %d MCP server processes", len(h.runningProcesses))

	for serverID, process := range h.runningProcesses {
		if err := process.Cmd.Process.Kill(); err != nil {
			log.Printf("Warning: failed to kill process for server %s: %v", serverID, err)
		}
		h.portAllocator.Release(process.Port)
	}

	h.runningProcesses = make(map[string]*ProcessInfo)
}

// monitorProcess monitors a process and cleans up if it exits
func (h *LocalProcessDeploymentHandler) monitorProcess(serverID string) {
	process := h.runningProcesses[serverID]
	if process == nil {
		return
	}

	// Wait for the process to exit
	err := process.Cmd.Wait()

	h.mutex.Lock()
	defer h.mutex.Unlock()

	// Process has exited, clean up
	if err != nil {
		log.Printf("MCP server process for %s exited with error: %v", serverID, err)
	} else {
		log.Printf("MCP server process for %s exited normally", serverID)
	}

	// Clean up
	h.portAllocator.Release(process.Port)
	delete(h.runningProcesses, serverID)
}

// LocalDeployRequest represents a local deployment request
type LocalDeployRequest struct {
	ServerID string            `json:"server_id"`
	EnvVars  map[string]string `json:"env_vars,omitempty"`
	Replicas int               `json:"replicas,omitempty"`
}

// LocalDeployResponse represents a local deployment response
type LocalDeployResponse struct {
	Message      string `json:"message"`
	DeploymentID string `json:"deployment_id"`
}

// DeployMCP handles MCP server deployment requests
func (h *LocalProcessDeploymentHandler) DeployMCP(c *gin.Context) {
	var req LocalDeployRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("Error decoding deployment request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	// Validate request
	if req.ServerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "server_id is required"})
		return
	}

	// Set defaults
	if req.Replicas <= 0 {
		req.Replicas = 1
	}

	// Deploy the server
	err := h.DeployMCPDirect(req.ServerID, req.EnvVars, req.Replicas)
	if err != nil {
		log.Printf("Failed to deploy MCP server %s: %v", req.ServerID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to deploy server: %v", err)})
		return
	}

	// Get process info for response
	processInfo, err := h.GetProcessInfo(req.ServerID)
	if err != nil {
		log.Printf("Failed to get process info for %s: %v", req.ServerID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Deployment succeeded but failed to get process info"})
		return
	}

	response := LocalDeployResponse{
		Message:      "MCP server deployed successfully",
		DeploymentID: fmt.Sprintf("local-%s-%d", req.ServerID, processInfo.Pid),
	}

	c.JSON(http.StatusOK, response)
}

// GetMCPConfig returns the configuration template for a server (gin handler)
func (h *LocalProcessDeploymentHandler) GetMCPConfig(c *gin.Context) {
	serverID := c.Param("serverId")
	// Remove leading slash if present
	serverID = strings.TrimPrefix(serverID, "/")

	// Get server from registry
	server, err := h.store.GetMCPServer(serverID)
	if err != nil {
		log.Printf("MCP server not found: %s", serverID)
		c.JSON(http.StatusNotFound, gin.H{"error": "MCP server not found"})
		return
	}

	if server.ConfigTemplate == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No configuration template available for this server"})
		return
	}

	c.JSON(http.StatusOK, server.ConfigTemplate)
}
