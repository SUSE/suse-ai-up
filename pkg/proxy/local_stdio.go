package proxy

import (
	"bufio"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/session"

	"github.com/gin-gonic/gin"
)

//go:embed src requirements.txt
var embeddedScripts embed.FS

// runningProcess represents a running MCP subprocess
type runningProcess struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	stderr  io.ReadCloser
	mutex   sync.Mutex
	tempDir string // for cleanup
}

// LocalStdioProxyPlugin handles local stdio MCP servers
type LocalStdioProxyPlugin struct {
	processes map[string]*runningProcess
	mutex     sync.RWMutex
}

// generateSessionID creates a unique session ID for LocalStdio adapters
func (p *LocalStdioProxyPlugin) generateSessionID(adapterName string) string {
	// Generate 8 random bytes for uniqueness
	bytes := make([]byte, 8)
	rand.Read(bytes)
	suffix := hex.EncodeToString(bytes)

	return fmt.Sprintf("stdio-%s-%d-%s", adapterName, time.Now().Unix(), suffix)
}

// isInitializeCall checks if the request is an initialize call
func (p *LocalStdioProxyPlugin) isInitializeCall(requestBody string) bool {
	return strings.Contains(requestBody, `"method": "initialize"`)
}

func NewLocalStdioProxyPlugin() *LocalStdioProxyPlugin {
	return &LocalStdioProxyPlugin{
		processes: make(map[string]*runningProcess),
	}
}

// getOrStartProcess gets the running process for an adapter, starting it if necessary
func (p *LocalStdioProxyPlugin) getOrStartProcess(adapter models.AdapterResource) (*runningProcess, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if proc, exists := p.processes[adapter.Name]; exists {
		return proc, nil
	}

	// Determine command and args
	var command string
	var args []string
	var tempDir string
	var mcpEnvVars map[string]string

	if len(adapter.MCPClientConfig.MCPServers) > 0 {
		for _, serverConfig := range adapter.MCPClientConfig.MCPServers {
			command = serverConfig.Command
			args = serverConfig.Args
			mcpEnvVars = serverConfig.Env
			break
		}
		tempDir = ""
	} else {
		// Legacy mode - extract scripts
		var err error
		tempDir, err = os.MkdirTemp("", "mcp-scripts-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp directory: %w", err)
		}

		if err := p.extractScripts(tempDir); err != nil {
			os.RemoveAll(tempDir)
			return nil, fmt.Errorf("failed to extract scripts: %w", err)
		}

		// Install requirements
		requirementsPath := fmt.Sprintf("%s/requirements.txt", tempDir)
		if _, err := os.Stat(requirementsPath); err == nil {
			installCmd := exec.Command("pip3", "install", "-r", requirementsPath)
			installCmd.Dir = tempDir
			if err := installCmd.Run(); err != nil {
				fmt.Printf("Warning: failed to install requirements: %v\n", err)
			}
		}

		scriptName := "main.py"
		if adapter.Command != "" {
			scriptName = adapter.Command
		}

		scriptPath := fmt.Sprintf("%s/src/%s", tempDir, scriptName)
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			os.RemoveAll(tempDir)
			return nil, fmt.Errorf("script %s not found", scriptName)
		}

		command = "python3"
		args = []string{scriptPath}
		if len(adapter.Args) > 0 {
			args = append(args, adapter.Args...)
		}
	}

	// Spawn the subprocess
	cmd := exec.Command(command, args...)
	if tempDir != "" {
		cmd.Dir = tempDir
	}

	cmd.Env = os.Environ()
	for k, v := range adapter.EnvironmentVariables {
		if k != "MCP_TRANSPORT" {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	// Add environment variables from MCPClientConfig
	for k, v := range mcpEnvVars {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
		return nil, fmt.Errorf("failed to get stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
		return nil, fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
		return nil, fmt.Errorf("failed to start subprocess: %w", err)
	}

	proc := &runningProcess{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		tempDir: tempDir,
	}

	p.processes[adapter.Name] = proc

	// Start a goroutine to monitor the process and filter security warnings
	go func() {
		// Start a goroutine to filter stderr output
		go func() {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				line := scanner.Text()
				// Filter out FastMCP security warnings that are expected in development
				if !strings.Contains(line, "Request filter disabled") &&
					!strings.Contains(line, "vulnerable to XSRF attacks") &&
					!strings.Contains(line, "CSRF") &&
					!strings.Contains(line, "XSRF") {
					log.Printf("[MCP_PROTOCOL] Adapter: %s | Message: %s", adapter.Name, line)
				}
			}
		}()

		err := cmd.Wait()
		if err != nil {
			log.Printf("[MCP_PROTOCOL] Adapter: %s | ERROR: Subprocess exited with error: %v", adapter.Name, err)
		} else {
			log.Printf("[MCP_PROTOCOL] Adapter: %s | INFO: Subprocess exited normally", adapter.Name)
		}
		// Remove from map when process exits
		p.mutex.Lock()
		delete(p.processes, adapter.Name)
		p.mutex.Unlock()
	}()

	return proc, nil
}

// sendMessage sends a JSON-RPC message to the subprocess and reads the response
func (p *LocalStdioProxyPlugin) sendMessage(proc *runningProcess, message string) (string, error) {
	proc.mutex.Lock()
	defer proc.mutex.Unlock()

	// Send the message
	if _, err := fmt.Fprintln(proc.stdin, message); err != nil {
		return "", fmt.Errorf("failed to send message to subprocess: %w", err)
	}

	// Read the response using bufio for line-based reading
	scanner := bufio.NewScanner(proc.stdout)
	if scanner.Scan() {
		response := scanner.Text()
		return response, nil
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[MCP_PROTOCOL] ERROR: Scanner error: %v", err)
		return "", fmt.Errorf("error reading from subprocess: %w", err)
	}

	log.Printf("[MCP_PROTOCOL] ERROR: No response from subprocess")
	return "", fmt.Errorf("no response from subprocess")
}

// extractScripts extracts the embedded scripts to the specified directory
func (p *LocalStdioProxyPlugin) extractScripts(destDir string) error {
	// Walk through the embedded filesystem and extract files
	return p.walkAndExtract(embeddedScripts, ".", destDir)
}

// walkAndExtract recursively walks the embedded FS and extracts files
func (p *LocalStdioProxyPlugin) walkAndExtract(fs embed.FS, srcDir, destDir string) error {
	entries, err := fs.ReadDir(srcDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := srcDir + "/" + entry.Name()
		destPath := destDir + "/" + entry.Name()

		if entry.IsDir() {
			// Create directory
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return err
			}
			// Recursively extract
			if err := p.walkAndExtract(fs, srcPath, destPath); err != nil {
				return err
			}
		} else {
			// Extract file
			data, err := fs.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(destPath, data, 0644); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *LocalStdioProxyPlugin) CanHandle(connectionType models.ConnectionType) bool {
	return connectionType == models.ConnectionTypeLocalStdio
}

func (p *LocalStdioProxyPlugin) ProxyRequest(c *gin.Context, adapter models.AdapterResource, sessionStore session.SessionStore) error {
	// Read the request body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return err
	}

	requestBodyStr := string(body)

	// Check if this is an initialize call - generate session ID if so
	var sessionID string
	if p.isInitializeCall(requestBodyStr) {
		sessionID = p.generateSessionID(adapter.Name)
		// Set session ID in response header
		c.Header("mcp-session-id", sessionID)
		// Register session in session store
		if err := sessionStore.SetWithDetails(sessionID, adapter.Name, "local", string(adapter.ConnectionType)); err != nil {
			log.Printf("[MCP_PROTOCOL] WARNING: Failed to register session %s: %v", sessionID, err)
		}
	} else {
		// For non-initialize calls, validate session ID
		requestSessionID := c.GetHeader("mcp-session-id")
		if requestSessionID == "" {
			requestSessionID = c.GetHeader("session-id")
		}

		if requestSessionID != "" {
			// Validate session exists and belongs to this adapter
			if details, err := sessionStore.GetDetails(requestSessionID); err != nil {
				log.Printf("[MCP_PROTOCOL] WARNING: Invalid session ID %s: %v", requestSessionID, err)
			} else if details.AdapterName != adapter.Name {
				log.Printf("[MCP_PROTOCOL] WARNING: Session ID %s belongs to different adapter %s, not %s",
					requestSessionID, details.AdapterName, adapter.Name)
			} else {
				// Update session activity
				if err := sessionStore.UpdateActivity(requestSessionID); err != nil {
					log.Printf("[MCP_PROTOCOL] WARNING: Failed to update session activity for %s: %v", requestSessionID, err)
				}
			}
		}
		// Note: We allow calls without session IDs for backward compatibility
	}

	// Get or start the persistent process
	proc, err := p.getOrStartProcess(adapter)
	if err != nil {
		log.Printf("[MCP_PROTOCOL] ERROR: Failed to get/start process: %v", err)
		return fmt.Errorf("failed to get/start process: %w", err)
	}

	// Send the message and get response
	response, err := p.sendMessage(proc, string(body))
	if err != nil {
		log.Printf("[MCP_PROTOCOL] ERROR: Failed to send message: %v", err)
		return err
	}

	// Return the response
	c.Header("Content-Type", "application/json")
	c.String(200, response)
	return nil
}

func (p *LocalStdioProxyPlugin) GetStatus(adapter models.AdapterResource) (models.AdapterStatus, error) {
	return models.AdapterStatus{ReplicaStatus: "Ready"}, nil
}

func (p *LocalStdioProxyPlugin) GetLogs(adapter models.AdapterResource) (string, error) {
	return "Local subprocess - logs not available", nil
}

func (p *LocalStdioProxyPlugin) Cleanup(adapterID string) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if proc, exists := p.processes[adapterID]; exists {
		// Close stdin to signal the process to exit
		proc.stdin.Close()
		// Wait for the process to finish
		proc.cmd.Wait()
		// Clean up temp directory if any
		if proc.tempDir != "" {
			os.RemoveAll(proc.tempDir)
		}
		delete(p.processes, adapterID)
	}
	return nil
}
