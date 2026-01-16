package discovery

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"suse-ai-up/pkg/models"
)

// DockerCommandExtractor extracts Docker commands from GitHub READMEs
type DockerCommandExtractor struct {
	httpClient *http.Client
}

// DockerConfig represents extracted Docker configuration
type DockerConfig struct {
	Image   string `json:"dockerImage"`
	Command string `json:"dockerCommand"`
	Source  string `json:"source"`
}

// NewDockerCommandExtractor creates a new extractor
func NewDockerCommandExtractor() *DockerCommandExtractor {
	return &DockerCommandExtractor{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ExtractFromGitHub extracts Docker commands from a GitHub repository README
func (e *DockerCommandExtractor) ExtractFromGitHub(ctx context.Context, repoURL string) (*DockerConfig, error) {
	// Extract owner/repo from GitHub URL
	repo, err := e.extractRepoFromURL(repoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to extract repo from URL %s: %w", repoURL, err)
	}

	// Fetch README content
	readmeURL := fmt.Sprintf("https://api.github.com/repos/%s/readme", repo)
	readme, err := e.fetchREADME(ctx, readmeURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch README for %s: %w", repo, err)
	}

	// Extract Docker commands
	config, err := e.extractDockerCommands(readme)
	if err != nil {
		return nil, fmt.Errorf("failed to extract Docker commands from %s: %w", repo, err)
	}

	if config != nil {
		config.Source = fmt.Sprintf("auto-extracted from %s", repoURL)
	}

	return config, nil
}

// extractRepoFromURL extracts owner/repo from GitHub URL
func (e *DockerCommandExtractor) extractRepoFromURL(url string) (string, error) {
	// Handle various GitHub URL formats
	patterns := []string{
		`github\.com/([^/]+/[^/]+)`,
		`github\.com/([^/]+/[^/]+)\.git`,
		`github\.com/([^/]+/[^/]+)/?$`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(url)
		if len(matches) > 1 {
			repo := strings.TrimSuffix(matches[1], "/")
			return repo, nil
		}
	}

	return "", fmt.Errorf("could not extract repo from URL: %s", url)
}

// fetchREADME fetches README content from GitHub API
func (e *DockerCommandExtractor) fetchREADME(ctx context.Context, apiURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", err
	}

	// Add GitHub API headers
	req.Header.Set("Accept", "application/vnd.github.v3.raw")
	req.Header.Set("User-Agent", "SUSE-AI-UP-Discovery/1.0")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("README not found")
		}
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// extractCommands parses README content for various command types
func (e *DockerCommandExtractor) extractCommands(content string) (*CommandConfig, error) {
	// Try different command types in order of preference
	if config, err := e.extractDockerCommands(content); err == nil && config != nil {
		return &CommandConfig{
			CommandType:   "docker",
			DockerImage:   config.Image,
			DockerCommand: config.Command,
		}, nil
	}

	if config, err := e.extractNpxCommands(content); err == nil && config != nil {
		return config, nil
	}

	if config, err := e.extractPythonCommands(content); err == nil && config != nil {
		return config, nil
	}

	if config, err := e.extractUvCommands(content); err == nil && config != nil {
		return config, nil
	}

	return nil, nil // No commands found
}

// CommandConfig represents extracted command configuration
type CommandConfig struct {
	CommandType   string   `json:"commandType"`
	BaseImage     string   `json:"baseImage,omitempty"`
	Command       string   `json:"command"`
	Args          []string `json:"args,omitempty"`
	DockerImage   string   `json:"dockerImage,omitempty"`
	DockerCommand string   `json:"dockerCommand,omitempty"`
	Source        string   `json:"source,omitempty"`
}

// extractDockerCommands parses README content for Docker commands
func (e *DockerCommandExtractor) extractDockerCommands(content string) (*DockerConfig, error) {
	// Look for docker run commands
	dockerRunPattern := regexp.MustCompile(`(?m)docker run[^\n]+mcp[^\n]*`)
	matches := dockerRunPattern.FindAllString(content, -1)

	if len(matches) == 0 {
		return nil, nil // No Docker commands found
	}

	// Use the first (most complete) command found
	command := strings.TrimSpace(matches[0])

	// Extract image name from command
	imagePattern := regexp.MustCompile(`docker run[^-]*(-[^i]*)*--rm[^-]*(-\w+\s+[^-]*)*\s+([^\s]+)`)
	imageMatches := imagePattern.FindStringSubmatch(command)

	var image string
	if len(imageMatches) > 3 {
		image = imageMatches[3]
	} else {
		// Fallback: extract last part that looks like an image
		parts := strings.Fields(command)
		for i := len(parts) - 1; i >= 0; i-- {
			part := parts[i]
			if strings.Contains(part, "/") || strings.Contains(part, ":") {
				image = part
				break
			}
		}
	}

	if image == "" {
		return nil, fmt.Errorf("could not extract image from command: %s", command)
	}

	// Check if this is an HTTP-capable server (has --host and --port arguments)
	if !strings.Contains(command, "--host") || !strings.Contains(command, "--port") {
		return nil, fmt.Errorf("server does not appear to support HTTP mode (missing --host or --port arguments)")
	}

	// Clean up the command for use in sidecar
	cleanCommand := e.cleanDockerCommand(command)

	return &DockerConfig{
		Image:   image,
		Command: cleanCommand,
	}, nil
}

// cleanDockerCommand removes docker run prefix and cleans up for sidecar use
func (e *DockerCommandExtractor) cleanDockerCommand(command string) string {
	// Remove "docker run" prefix
	cleaned := strings.TrimPrefix(command, "docker run")

	// Remove common flags that don't make sense in sidecar context
	cleaned = regexp.MustCompile(`-p\s+\S+`).ReplaceAllString(cleaned, "")
	cleaned = regexp.MustCompile(`--rm`).ReplaceAllString(cleaned, "")
	cleaned = regexp.MustCompile(`-i`).ReplaceAllString(cleaned, "")
	cleaned = regexp.MustCompile(`--name\s+\S+`).ReplaceAllString(cleaned, "")

	// Clean up extra whitespace
	cleaned = regexp.MustCompile(`\s+`).ReplaceAllString(cleaned, " ")
	cleaned = strings.TrimSpace(cleaned)

	return cleaned
}

// extractNpxCommands parses README content for npx commands
func (e *DockerCommandExtractor) extractNpxCommands(content string) (*CommandConfig, error) {
	// Look for npx commands
	npxPattern := regexp.MustCompile(`(?m)npx\s+(@?[a-zA-Z0-9/-]+)([^\n]*)`)
	matches := npxPattern.FindAllStringSubmatch(content, -1)

	if len(matches) == 0 {
		return nil, nil // No npx commands found
	}

	// Use the first command found
	match := matches[0]
	packageName := strings.TrimSpace(match[1])
	args := strings.TrimSpace(match[2])

	// Parse arguments
	var argList []string
	if args != "" {
		argList = strings.Fields(args)
	}

	return &CommandConfig{
		CommandType: "npx",
		BaseImage:   "registry.suse.com/bci/nodejs:22",
		Command:     packageName,
		Args:        argList,
	}, nil
}

// extractPythonCommands parses README content for python commands
func (e *DockerCommandExtractor) extractPythonCommands(content string) (*CommandConfig, error) {
	// Look for python commands
	pythonPattern := regexp.MustCompile(`(?m)python\s+([a-zA-Z0-9_.-]+\.py)([^\n]*)`)
	matches := pythonPattern.FindAllStringSubmatch(content, -1)

	if len(matches) == 0 {
		return nil, nil // No python commands found
	}

	// Use the first command found
	match := matches[0]
	scriptName := strings.TrimSpace(match[1])
	args := strings.TrimSpace(match[2])

	// Parse arguments
	var argList []string
	if args != "" {
		argList = strings.Fields(args)
	}

	return &CommandConfig{
		CommandType: "python",
		BaseImage:   "registry.suse.com/bci/python:3.12",
		Command:     scriptName,
		Args:        argList,
	}, nil
}

// extractUvCommands parses README content for uv commands
func (e *DockerCommandExtractor) extractUvCommands(content string) (*CommandConfig, error) {
	// Look for uv run commands
	uvPattern := regexp.MustCompile(`(?m)uv\s+run\s+([a-zA-Z0-9_.-]+)([^\n]*)`)
	matches := uvPattern.FindAllStringSubmatch(content, -1)

	if len(matches) == 0 {
		return nil, nil // No uv commands found
	}

	// Use the first command found
	match := matches[0]
	commandName := strings.TrimSpace(match[1])
	args := strings.TrimSpace(match[2])

	// Parse arguments
	var argList []string
	if args != "" {
		argList = strings.Fields(args)
	}

	return &CommandConfig{
		CommandType: "uv",
		BaseImage:   "registry.suse.com/bci/python:3.12",
		Command:     commandName,
		Args:        argList,
	}, nil
}

// UpdateRegistryWithCommands updates the registry with command configurations for stdio servers
func (e *DockerCommandExtractor) UpdateRegistryWithCommands(ctx context.Context, registry *[]models.MCPServer) error {
	updated := 0

	for i, server := range *registry {
		// Only process stdio servers that don't already have sidecar config and have GitHub documentation
		if e.hasStdioPackage(server) && server.Meta["sidecarConfig"] == nil && e.hasGitHubDocumentation(server) {
			if config, err := e.extractCommandConfigForServer(ctx, server); err == nil && config != nil {
				// Add sidecar config to server metadata
				if (*registry)[i].Meta == nil {
					(*registry)[i].Meta = make(map[string]interface{})
				}

				sidecarConfig := map[string]interface{}{
					"commandType": config.CommandType,
					"command":     config.Command,
					"args":        config.Args,
					"source":      "auto-extracted from GitHub",
					"lastUpdated": time.Now().Format(time.RFC3339),
				}

				// Add type-specific fields
				if config.BaseImage != "" {
					sidecarConfig["baseImage"] = config.BaseImage
				}
				if config.DockerImage != "" {
					sidecarConfig["dockerImage"] = config.DockerImage
				}
				if config.DockerCommand != "" {
					sidecarConfig["dockerCommand"] = config.DockerCommand
				}

				(*registry)[i].Meta["sidecarConfig"] = sidecarConfig

				updated++
				fmt.Printf("✅ Updated %s with %s config\n", server.ID, config.CommandType)
			}
		}
	}

	fmt.Printf("📊 Updated %d/%d servers with command configurations\n", updated, len(*registry))
	return nil
}

// hasStdioPackage checks if server has stdio packages
func (e *DockerCommandExtractor) hasStdioPackage(server models.MCPServer) bool {
	for _, pkg := range server.Packages {
		if pkg.RegistryType == "stdio" {
			return true
		}
	}
	return false
}

// extractDockerConfigForServer extracts Docker config for a specific server
func (e *DockerCommandExtractor) extractDockerConfigForServer(ctx context.Context, server models.MCPServer) (*DockerConfig, error) {
	// Get documentation URL
	docURL, ok := server.Meta["documentation"].(string)
	if !ok {
		return nil, fmt.Errorf("no documentation URL found")
	}

	// Check if it's a GitHub URL
	if strings.Contains(docURL, "github.com") {
		return e.ExtractFromGitHub(ctx, docURL)
	}

	// For non-GitHub URLs, we can't automatically extract
	return nil, fmt.Errorf("non-GitHub documentation URL: %s", docURL)
}

// hasGitHubDocumentation checks if server has GitHub documentation URL
func (e *DockerCommandExtractor) hasGitHubDocumentation(server models.MCPServer) bool {
	if server.Meta == nil {
		return false
	}

	docURL, ok := server.Meta["documentation"].(string)
	if !ok {
		return false
	}

	return strings.Contains(docURL, "github.com")
}

// extractCommandConfigForServer extracts command config for a specific server
func (e *DockerCommandExtractor) extractCommandConfigForServer(ctx context.Context, server models.MCPServer) (*CommandConfig, error) {
	// Get documentation URL
	docURL, ok := server.Meta["documentation"].(string)
	if !ok {
		return nil, fmt.Errorf("no documentation URL found")
	}

	// Only process GitHub URLs
	if !strings.Contains(docURL, "github.com") {
		return nil, fmt.Errorf("non-GitHub documentation URL: %s", docURL)
	}

	// Extract repo from GitHub URL
	repo, err := e.extractRepoFromURL(docURL)
	if err != nil {
		return nil, fmt.Errorf("failed to extract repo from URL %s: %w", docURL, err)
	}

	// Fetch README content
	readmeURL := fmt.Sprintf("https://api.github.com/repos/%s/readme", repo)
	readme, err := e.fetchREADME(ctx, readmeURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch README for %s: %w", repo, err)
	}

	// Extract commands
	config, err := e.extractCommands(readme)
	if err != nil {
		return nil, fmt.Errorf("failed to extract commands from %s: %w", repo, err)
	}

	if config != nil {
		config.Source = fmt.Sprintf("auto-extracted from %s", docURL)
	}

	return config, nil
}

// ValidateDockerConfig validates that a Docker config is usable
func (e *DockerCommandExtractor) ValidateDockerConfig(ctx context.Context, config *DockerConfig) error {
	if config.Image == "" {
		return fmt.Errorf("empty Docker image")
	}

	if config.Command == "" {
		return fmt.Errorf("empty Docker command")
	}

	// Basic validation - check if image name looks reasonable
	if !strings.Contains(config.Image, "/") {
		return fmt.Errorf("image name should contain registry/organization: %s", config.Image)
	}

	return nil
}
