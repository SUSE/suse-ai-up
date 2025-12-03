package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"suse-ai-up/internal/config"
	"suse-ai-up/pkg/models"
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
	spawningConfig   *config.SpawningConfig
}

// NewLocalProcessDeploymentHandler creates a new local process deployment handler
func NewLocalProcessDeploymentHandler(store MCPServerStore, minPort, maxPort int, spawningConfig *config.SpawningConfig) *LocalProcessDeploymentHandler {
	return &LocalProcessDeploymentHandler{
		runningProcesses: make(map[string]*ProcessInfo),
		portAllocator:    NewPortAllocator(minPort, maxPort),
		store:            store,
		spawningConfig:   spawningConfig,
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

	// Perform security validation
	if err := h.validateServerSecurity(server); err != nil {
		h.portAllocator.Release(port)
		return fmt.Errorf("security validation failed: %v", err)
	}

	// Handle dynamic dependencies for registry servers
	if err := h.installServerDependencies(server); err != nil {
		h.portAllocator.Release(port)
		return fmt.Errorf("failed to install server dependencies: %v", err)
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
		fmt.Sprintf("MCP_PORT=%d", port),
		fmt.Sprintf("TOOLS_CONFIG=%s", string(toolsJSON)),
		fmt.Sprintf("BEARER_TOKEN=%s", tokenStr),
		fmt.Sprintf("SERVER_NAME=%s", serverID),
		"TRANSPORT_MODE=http",
	}

	// Add server-specific environment variables
	for key, value := range server.ConfigTemplate.Env {
		if key != "PORT" && key != "MCP_PORT" && key != "TOOLS_CONFIG" && key != "BEARER_TOKEN" { // Don't override our values
			env = append(env, fmt.Sprintf("%s=%s", key, value))
		}
	}

	// Add user-provided environment variables
	for key, value := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	// Start the MCP server process with resource limits
	cmd := exec.Command("npx", "tsx", "templates/virtualmcp-server.ts", "--transport", "http")
	cmd.Env = env
	cmd.Dir = "." // Run from current directory

	// Set resource limits to prevent runaway processes
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // Create new process group for better control
	}

	// Set environment variables for resource limits (Node.js specific)
	if server.ConfigTemplate.ResourceLimits != nil && server.ConfigTemplate.ResourceLimits.Memory != "" {
		// Parse memory limit (e.g., "256Mi" -> 256)
		memoryLimit := h.parseMemoryLimit(server.ConfigTemplate.ResourceLimits.Memory)
		if memoryLimit > 0 {
			env = append(env, fmt.Sprintf("NODE_OPTIONS=--max-old-space-size=%d", memoryLimit))
		} else {
			// Fallback to default
			env = append(env, "NODE_OPTIONS=--max-old-space-size=256")
		}
	} else {
		// Use configured default
		defaultMemoryMB := h.parseMemoryLimit(h.spawningConfig.DefaultMemory)
		if defaultMemoryMB > 0 {
			env = append(env, fmt.Sprintf("NODE_OPTIONS=--max-old-space-size=%d", defaultMemoryMB))
		} else {
			env = append(env, "NODE_OPTIONS=--max-old-space-size=256")
		}
	}
	cmd.Env = env

	// Capture output for logging - create log files for debugging
	logDir := "/opt/mcp-servers/logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("Warning: Failed to create log directory: %v", err)
	}

	stdoutFile := fmt.Sprintf("%s/%s.stdout.log", logDir, serverID)
	stderrFile := fmt.Sprintf("%s/%s.stderr.log", logDir, serverID)

	if stdout, err := os.Create(stdoutFile); err == nil {
		cmd.Stdout = stdout
		defer stdout.Close()
	}

	if stderr, err := os.Create(stderrFile); err == nil {
		cmd.Stderr = stderr
		defer stderr.Close()
	}

	// Enhanced logging for debugging
	log.Printf("Registry: Starting MCP server process for %s on port %d", serverID, port)
	log.Printf("Registry: Command: %s %s", cmd.Path, strings.Join(cmd.Args, " "))
	log.Printf("Registry: Environment variables: %d total", len(env))
	for _, envVar := range env {
		if strings.Contains(envVar, "TOKEN") || strings.Contains(envVar, "SECRET") {
			log.Printf("Registry:   %s=***", strings.Split(envVar, "=")[0])
		} else {
			log.Printf("Registry:   %s", envVar)
		}
	}
	log.Printf("Registry: Working directory: %s", cmd.Dir)
	log.Printf("Registry: Log files: stdout=%s, stderr=%s", stdoutFile, stderrFile)

	if err := cmd.Start(); err != nil {
		log.Printf("Registry: Failed to start MCP server process: %v", err)
		h.portAllocator.Release(port)
		return fmt.Errorf("failed to start MCP server process (check logs at %s): %v", stderrFile, err)
	}

	log.Printf("Registry: Successfully started MCP server process with PID: %d", cmd.Process.Pid)

	// Wait a bit for the server to start up
	time.Sleep(2 * time.Second)

	// Health check the server
	healthURL := fmt.Sprintf("http://localhost:%d/health", port)
	log.Printf("Performing health check on %s", healthURL)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(healthURL)
	if err != nil {
		log.Printf("Health check failed for server %s: %v", serverID, err)
		// Don't fail the deployment, just log the warning
	} else {
		resp.Body.Close()
		if resp.StatusCode == 200 {
			log.Printf("Health check passed for server %s", serverID)
		} else {
			log.Printf("Health check failed for server %s with status: %d", serverID, resp.StatusCode)
		}
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

// validateServerSecurity performs basic security validation on MCP server configuration
func (h *LocalProcessDeploymentHandler) validateServerSecurity(server *models.MCPServer) error {
	// Check for potentially dangerous commands in metadata
	if server.Meta != nil {
		if cmd, ok := server.Meta["command"]; ok {
			if cmdStr, ok := cmd.(string); ok {
				dangerousCommands := []string{"rm", "rmdir", "del", "format", "fdisk", "mkfs", "dd", "wget", "curl"}
				for _, dangerous := range dangerousCommands {
					if strings.Contains(strings.ToLower(cmdStr), dangerous) {
						return fmt.Errorf("potentially dangerous command detected: %s", dangerous)
					}
				}
			}
		}

		// Check for suspicious environment variables
		if env, ok := server.Meta["env"]; ok {
			if envMap, ok := env.(map[string]interface{}); ok {
				for key, value := range envMap {
					if _, ok := value.(string); ok {
						// Check for suspicious environment variables
						if strings.Contains(strings.ToLower(key), "password") ||
							strings.Contains(strings.ToLower(key), "secret") ||
							strings.Contains(strings.ToLower(key), "token") {
							log.Printf("Registry: Warning - server %s has sensitive environment variable: %s", server.ID, key)
						}
					}
				}
			}
		}
	}

	// Validate package identifiers for suspicious content
	if len(server.Packages) > 0 {
		for _, pkg := range server.Packages {
			// Check for suspicious package names
			suspicious := []string{"malware", "virus", "trojan", "exploit", "hack", "attack"}
			for _, susp := range suspicious {
				if strings.Contains(strings.ToLower(pkg.Identifier), susp) {
					return fmt.Errorf("suspicious package identifier detected: %s", pkg.Identifier)
				}
			}
		}
	}

	return nil
}

// installServerDependencies installs dynamic dependencies for registry MCP servers
func (h *LocalProcessDeploymentHandler) installServerDependencies(server *models.MCPServer) error {
	// Check for Python requirements in server metadata
	if server.Meta != nil {
		if requirements, ok := server.Meta["requirements.txt"]; ok {
			if reqText, ok := requirements.(string); ok {
				log.Printf("Registry: Installing Python dependencies for server %s", server.ID)
				if err := h.installPythonRequirements(server.ID, reqText); err != nil {
					log.Printf("Registry: Failed to install Python requirements for server %s: %v", server.ID, err)
					return fmt.Errorf("failed to install Python requirements: %v", err)
				}
			}
		}

		if packageJson, ok := server.Meta["package.json"]; ok {
			if pkgText, ok := packageJson.(string); ok {
				log.Printf("Registry: Installing Node.js dependencies for server %s", server.ID)
				if err := h.installNodeDependencies(server.ID, pkgText); err != nil {
					log.Printf("Registry: Failed to install Node.js dependencies for server %s: %v", server.ID, err)
					return fmt.Errorf("failed to install Node.js dependencies: %v", err)
				}
			}
		}
	}

	// Check packages for dependency information
	if len(server.Packages) > 0 {
		for _, pkg := range server.Packages {
			// Handle Python packages
			if pkg.RegistryType == "pypi" || strings.Contains(strings.ToLower(pkg.Identifier), "python") {
				log.Printf("Registry: Detected Python package for server %s: %s", server.ID, pkg.Identifier)
				// Extract package name and install
				if err := h.installPythonPackage(server.ID, pkg.Identifier); err != nil {
					log.Printf("Registry: Failed to install Python package %s: %v", pkg.Identifier, err)
					return fmt.Errorf("failed to install Python package %s: %v", pkg.Identifier, err)
				}
			}

			// Handle Node.js packages
			if pkg.RegistryType == "npm" || strings.Contains(strings.ToLower(pkg.Identifier), "node") || strings.Contains(strings.ToLower(pkg.Identifier), "typescript") {
				log.Printf("Registry: Detected Node.js package for server %s: %s", server.ID, pkg.Identifier)
				if err := h.installNpmPackage(server.ID, pkg.Identifier); err != nil {
					log.Printf("Registry: Failed to install npm package %s: %v", pkg.Identifier, err)
					return fmt.Errorf("failed to install npm package %s: %v", pkg.Identifier, err)
				}
			}
		}
	}

	return nil
}

// installPythonRequirements installs Python packages from a requirements.txt string
func (h *LocalProcessDeploymentHandler) installPythonRequirements(serverID, requirementsTxt string) error {
	// Create a temporary requirements file
	tempDir := fmt.Sprintf("/tmp/mcp-%s", serverID)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	reqFile := fmt.Sprintf("%s/requirements.txt", tempDir)
	if err := os.WriteFile(reqFile, []byte(requirementsTxt), 0644); err != nil {
		return fmt.Errorf("failed to write requirements file: %v", err)
	}

	// Install requirements using pip
	cmd := exec.Command("pip3", "install", "--quiet", "--no-cache-dir", "-r", reqFile)
	cmd.Env = append(os.Environ(), "PYTHONPATH=/opt/venv/lib/python3.*/site-packages")

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Registry: pip install output: %s", string(output))
		return fmt.Errorf("pip install failed: %v", err)
	}

	log.Printf("Registry: Successfully installed Python requirements for server %s", serverID)
	return nil
}

// installNodeDependencies installs Node.js dependencies from a package.json string
func (h *LocalProcessDeploymentHandler) installNodeDependencies(serverID, packageJson string) error {
	// Create a temporary directory for the package
	tempDir := fmt.Sprintf("/tmp/mcp-node-%s", serverID)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Write package.json
	packageFile := fmt.Sprintf("%s/package.json", tempDir)
	if err := os.WriteFile(packageFile, []byte(packageJson), 0644); err != nil {
		return fmt.Errorf("failed to write package.json: %v", err)
	}

	// Install dependencies using npm
	cmd := exec.Command("npm", "install", "--silent")
	cmd.Dir = tempDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Registry: npm install output: %s", string(output))
		return fmt.Errorf("npm install failed: %v", err)
	}

	log.Printf("Registry: Successfully installed Node.js dependencies for server %s", serverID)
	return nil
}

// installPythonPackage installs a single Python package
func (h *LocalProcessDeploymentHandler) installPythonPackage(serverID, packageSpec string) error {
	cmd := exec.Command("pip3", "install", "--quiet", "--no-cache-dir", packageSpec)
	cmd.Env = append(os.Environ(), "PYTHONPATH=/opt/venv/lib/python3.*/site-packages")

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Registry: pip install output for %s: %s", packageSpec, string(output))
		return fmt.Errorf("failed to install Python package %s: %v", packageSpec, err)
	}

	log.Printf("Registry: Successfully installed Python package %s for server %s", packageSpec, serverID)
	return nil
}

// installNpmPackage installs a single npm package
func (h *LocalProcessDeploymentHandler) installNpmPackage(serverID, packageSpec string) error {
	cmd := exec.Command("npm", "install", "-g", "--silent", packageSpec)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Registry: npm install output for %s: %s", packageSpec, string(output))
		return fmt.Errorf("failed to install npm package %s: %v", packageSpec, err)
	}

	log.Printf("Registry: Successfully installed npm package %s for server %s", packageSpec, serverID)
	return nil
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

// parseMemoryLimit parses memory limit string (e.g., "256Mi", "1Gi") to MB
func (h *LocalProcessDeploymentHandler) parseMemoryLimit(memoryStr string) int {
	if memoryStr == "" {
		return 0
	}

	// Handle different units
	memoryStr = strings.ToLower(memoryStr)
	var multiplier int

	if strings.HasSuffix(memoryStr, "gi") {
		multiplier = 1024 // Gi to Mi
		memoryStr = strings.TrimSuffix(memoryStr, "gi")
	} else if strings.HasSuffix(memoryStr, "mi") {
		multiplier = 1 // Already in Mi
		memoryStr = strings.TrimSuffix(memoryStr, "mi")
	} else if strings.HasSuffix(memoryStr, "g") {
		multiplier = 1024 // G to Mi
		memoryStr = strings.TrimSuffix(memoryStr, "g")
	} else if strings.HasSuffix(memoryStr, "m") {
		multiplier = 1 // Already in M
		memoryStr = strings.TrimSuffix(memoryStr, "m")
	} else {
		// Assume Mi if no unit
		multiplier = 1
	}

	if value, err := strconv.Atoi(memoryStr); err == nil {
		return value * multiplier
	}

	return 0
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
