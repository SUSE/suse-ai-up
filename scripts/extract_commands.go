package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ServerConfig represents the YAML structure
type ServerConfig struct {
	Name   string                 `yaml:"name"`
	Image  string                 `yaml:"image"`
	Type   string                 `yaml:"type"`
	Meta   map[string]interface{} `yaml:"meta"`
	About  map[string]interface{} `yaml:"about"`
	Source map[string]interface{} `yaml:"source"`
	Config map[string]interface{} `yaml:"config"`
}

// GitHubContent represents GitHub API response for file content
type GitHubContent struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("Usage: go run extract_commands.go <yaml-file>")
	}

	yamlFile := os.Args[1]

	// Load current YAML
	data, err := os.ReadFile(yamlFile)
	if err != nil {
		log.Fatalf("Failed to read YAML file: %v", err)
	}

	var servers []ServerConfig
	if err := yaml.Unmarshal(data, &servers); err != nil {
		log.Fatalf("Failed to parse YAML: %v", err)
	}

	fmt.Printf("📊 Processing %d servers...\n", len(servers))

	// Process each server
	var updatedServers []ServerConfig
	removedCount := 0

	for _, server := range servers {
		config, err := extractCommandForServer(server)
		if err != nil {
			fmt.Printf("⚠️  Failed to extract command for %s: %v\n", server.Name, err)
			removedCount++
			continue
		}

		if config == nil {
			fmt.Printf("🚫 No executable command found for %s, removing\n", server.Name)
			removedCount++
			continue
		}

		// Add sidecar config to server
		if server.Meta == nil {
			server.Meta = make(map[string]interface{})
		}
		server.Meta["sidecarConfig"] = config
		updatedServers = append(updatedServers, server)
	}

	fmt.Printf("✅ Processed: %d kept, %d removed\n", len(updatedServers), removedCount)

	// Write updated YAML
	updatedData, err := yaml.Marshal(updatedServers)
	if err != nil {
		log.Fatalf("Failed to marshal updated YAML: %v", err)
	}

	if err := os.WriteFile(yamlFile, updatedData, 0644); err != nil {
		log.Fatalf("Failed to write updated YAML: %v", err)
	}

	fmt.Printf("🎉 Successfully updated %s with embedded commands\n", yamlFile)
}

func extractCommandForServer(server ServerConfig) (map[string]interface{}, error) {
	// Get project URL
	source, ok := server.Source["project"].(string)
	if !ok {
		return nil, fmt.Errorf("no project URL found")
	}

	// Parse GitHub URL
	owner, repo, err := parseGitHubURL(source)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GitHub URL: %w", err)
	}

	// Fetch README content
	content, err := fetchREADME(owner, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch README: %w", err)
	}

	// Try docker commands first
	if config := extractDockerCommand(content, server.Image); config != nil {
		return config, nil
	}

	// Try UV commands
	if config := extractUVCommand(content); config != nil {
		return config, nil
	}

	// Try NPX commands
	if config := extractNPXCommand(content); config != nil {
		return config, nil
	}

	// Try Python commands
	if config := extractPythonCommand(content); config != nil {
		return config, nil
	}

	// No commands found
	return nil, nil
}

func parseGitHubURL(url string) (owner, repo string, err error) {
	// Handle URLs like https://github.com/owner/repo
	if !strings.Contains(url, "github.com") {
		return "", "", fmt.Errorf("not a GitHub URL: %s", url)
	}

	parts := strings.Split(strings.Trim(url, "/"), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid GitHub URL format: %s", url)
	}

	// Find github.com in the parts
	githubIndex := -1
	for i, part := range parts {
		if part == "github.com" && i+2 < len(parts) {
			githubIndex = i
			break
		}
	}

	if githubIndex == -1 {
		return "", "", fmt.Errorf("could not parse GitHub URL: %s", url)
	}

	owner = parts[githubIndex+1]
	repo = parts[githubIndex+2]

	// Remove .git suffix if present
	repo = strings.TrimSuffix(repo, ".git")

	return owner, repo, nil
}

func fetchREADME(owner, repo string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/readme", owner, repo)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	// Add User-Agent to avoid rate limiting
	req.Header.Set("User-Agent", "mcp-server-extractor/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var content GitHubContent
	if err := json.Unmarshal(body, &content); err != nil {
		return "", err
	}

	if content.Encoding != "base64" {
		return "", fmt.Errorf("unexpected encoding: %s", content.Encoding)
	}

	decoded, err := base64.StdEncoding.DecodeString(content.Content)
	if err != nil {
		return "", err
	}

	return string(decoded), nil
}

func extractDockerCommand(content, image string) map[string]interface{} {
	// Look for docker run commands
	dockerPatterns := []string{
		`docker run.*(?:mcp|MCP).*server`,
		`docker run.*--name.*mcp`,
		`docker run.*-p.*\d+:\d+.*mcp`,
		`docker run.*mcp`,
	}

	for _, pattern := range dockerPatterns {
		re := regexp.MustCompile(`(?m)` + pattern)
		matches := re.FindAllString(content, -1)

		for _, match := range matches {
			// Clean up the command
			cmd := strings.TrimSpace(match)

			// Skip if it's just an example or template
			if strings.Contains(cmd, "<") || strings.Contains(cmd, "your") || strings.Contains(cmd, "example") {
				continue
			}

			// Extract arguments (everything after "docker run")
			parts := strings.Fields(cmd)
			if len(parts) >= 3 && parts[0] == "docker" && parts[1] == "run" {
				args := parts[2:]

				// If image is not in args, append it
				if image != "" && !containsImage(args, image) {
					// Try to find a placeholder or append at the end
					found := false
					for i, arg := range args {
						if strings.Contains(arg, "mcp/") || strings.Contains(arg, "ghcr.io") || strings.Contains(arg, "registry.") {
							args[i] = image
							found = true
							break
						}
					}
					if !found {
						args = append(args, image)
					}
				}

				return map[string]interface{}{
					"commandType": "docker",
					"command":     "run",
					"args":        args,
					"baseImage":   "registry.suse.com/bci/python:3.12",
					"source":      "pre-extracted",
					"lastUpdated": time.Now().Format(time.RFC3339),
				}
			}
		}
	}

	return nil
}

func extractUVCommand(content string) map[string]interface{} {
	patterns := []string{
		`uv run.*(?:mcp|MCP).*server`,
		`uv run.*mcp`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(`(?m)` + pattern)
		matches := re.FindAllString(content, -1)

		for _, match := range matches {
			cmd := strings.TrimSpace(match)
			if strings.Contains(cmd, "<") || strings.Contains(cmd, "your") {
				continue
			}

			parts := strings.Fields(cmd)
			if len(parts) >= 3 && parts[0] == "uv" && parts[1] == "run" {
				args := parts[2:]

				return map[string]interface{}{
					"commandType": "uv",
					"command":     "run",
					"args":        args,
					"baseImage":   "registry.suse.com/bci/python:3.12",
					"source":      "pre-extracted",
					"lastUpdated": time.Now().Format(time.RFC3339),
				}
			}
		}
	}

	return nil
}

func extractNPXCommand(content string) map[string]interface{} {
	patterns := []string{
		`npx.*@modelcontextprotocol.*server`,
		`npx.*mcp`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(`(?m)` + pattern)
		matches := re.FindAllString(content, -1)

		for _, match := range matches {
			cmd := strings.TrimSpace(match)
			if strings.Contains(cmd, "<") || strings.Contains(cmd, "your") {
				continue
			}

			parts := strings.Fields(cmd)
			if len(parts) >= 2 && parts[0] == "npx" {
				args := parts[1:]

				return map[string]interface{}{
					"commandType": "npx",
					"command":     "npx",
					"args":        args,
					"baseImage":   "registry.suse.com/bci/nodejs:22",
					"source":      "pre-extracted",
					"lastUpdated": time.Now().Format(time.RFC3339),
				}
			}
		}
	}

	return nil
}

func extractPythonCommand(content string) map[string]interface{} {
	patterns := []string{
		`python -m.*(?:mcp|MCP).*server`,
		`python -m.*mcp`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(`(?m)` + pattern)
		matches := re.FindAllString(content, -1)

		for _, match := range matches {
			cmd := strings.TrimSpace(match)
			if strings.Contains(cmd, "<") || strings.Contains(cmd, "your") {
				continue
			}

			parts := strings.Fields(cmd)
			if len(parts) >= 3 && parts[0] == "python" && parts[1] == "-m" {
				args := parts[2:]

				return map[string]interface{}{
					"commandType": "python",
					"command":     "-m",
					"args":        args,
					"baseImage":   "registry.suse.com/bci/python:3.12",
					"source":      "pre-extracted",
					"lastUpdated": time.Now().Format(time.RFC3339),
				}
			}
		}
	}

	return nil
}

func containsImage(args []string, image string) bool {
	for _, arg := range args {
		if arg == image || strings.Contains(arg, image) {
			return true
		}
	}
	return false
}
