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

func main() {
	if len(os.Args) != 2 {
		log.Fatal("Usage: go run extract_github_commands.go <yaml-file>")
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

	fmt.Printf("📊 Processing %d servers for GitHub command extraction...\n", len(servers))

	// Process each server
	var updatedServers []ServerConfig
	removedCount := 0

	for _, server := range servers {
		// Skip archived servers
		if title, ok := server.About["title"].(string); ok && strings.Contains(title, "(Archived)") {
			fmt.Printf("🚫 Skipping archived server: %s\n", server.Name)
			removedCount++
			continue
		}

		config := extractGitHubCommandForServer(server)
		if config == nil {
			fmt.Printf("🚫 No GitHub command config found for %s, removing\n", server.Name)
			removedCount++
			continue
		}

		// Add sidecar config to server
		if server.Meta == nil {
			server.Meta = make(map[string]interface{})
		}
		server.Meta["sidecarConfig"] = config
		updatedServers = append(updatedServers, server)
		fmt.Printf("✅ Added GitHub command config for %s\n", server.Name)
	}

	fmt.Printf("📊 Processed: %d kept, %d removed\n", len(updatedServers), removedCount)

	// Write updated YAML
	updatedData, err := yaml.Marshal(updatedServers)
	if err != nil {
		log.Fatalf("Failed to marshal updated YAML: %v", err)
	}

	if err := os.WriteFile(yamlFile, updatedData, 0644); err != nil {
		log.Fatalf("Failed to write updated YAML: %v", err)
	}

	fmt.Printf("🎉 Successfully updated %s with GitHub command configurations\n", yamlFile)
}

func extractGitHubCommandForServer(server ServerConfig) map[string]interface{} {
	// Get project URL
	source, ok := server.Source["project"].(string)
	if !ok {
		return nil
	}

	// Parse GitHub URL
	owner, repo, err := parseGitHubURL(source)
	if err != nil {
		return nil
	}

	// Fetch README content
	content, err := fetchREADME(owner, repo)
	if err != nil {
		return nil
	}

	// Look for the specific GitHub JSON structure
	return extractGitHubJSONCommand(content, server.Image)
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

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	// Add User-Agent and GitHub token for authentication
	req.Header.Set("User-Agent", "mcp-server-extractor/1.0")
	req.Header.Set("Authorization", "token ghp_1234567890abcdef") // Placeholder - will be replaced with actual token
	req.Header.Set("Accept", "application/vnd.github.v3+json")

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

	var content struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}

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

func extractGitHubJSONCommand(content, image string) map[string]interface{} {
	// Look for MCP client configuration JSON structures with command, args, and optional env

	// Pattern to match JSON objects with command and args fields
	// This looks for: { "command": "...", "args": [...], "env": {...} }
	pattern := `\{\s*"command"\s*:\s*"([^"]+)"\s*,\s*"args"\s*:\s*\[([^\]]*)\]`
	re := regexp.MustCompile(`(?s)` + pattern)

	matches := re.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		command := strings.TrimSpace(match[1])
		argsStr := strings.TrimSpace(match[2])

		// Parse args array
		args, err := parseJSONArray(argsStr)
		if err != nil {
			continue
		}

		// Skip if args doesn't look like a valid command (should start with "run" for docker, etc.)
		if len(args) == 0 {
			continue
		}

		// Determine command type and base image
		commandType := command
		baseImage := "registry.suse.com/bci/python:3.12"

		if command == "docker" {
			commandType = "docker"
			baseImage = "registry.suse.com/bci/python:3.12"
			// For docker commands, args should typically start with "run"
			if len(args) > 0 && args[0] != "run" {
				continue
			}
		} else if command == "npx" {
			commandType = "npx"
			baseImage = "registry.suse.com/bci/nodejs:22"
		} else if strings.HasPrefix(command, "uv") {
			commandType = "uv"
			baseImage = "registry.suse.com/bci/python:3.12"
		} else if command == "python" || strings.HasPrefix(command, "python") {
			commandType = "python"
			baseImage = "registry.suse.com/bci/python:3.12"
		}

		// If image is specified in args for docker, use it
		if command == "docker" && image != "" && !containsImage(args, image) {
			// Try to replace placeholder or append
			found := false
			for i, arg := range args {
				if strings.Contains(arg, "mcp/") || strings.Contains(arg, "ghcr.io") || strings.Contains(arg, "docker.io") {
					args[i] = image
					found = true
					break
				}
			}
			if !found && len(args) > 1 {
				// Replace the last argument if it looks like an image
				lastArg := args[len(args)-1]
				if strings.Contains(lastArg, "/") || strings.Contains(lastArg, ":") {
					args[len(args)-1] = image
				}
			}
		}

		return map[string]interface{}{
			"commandType": commandType,
			"command":     command,
			"args":        args,
			"baseImage":   baseImage,
			"source":      "mcp-client-config-extracted",
			"lastUpdated": time.Now().Format(time.RFC3339),
		}
	}

	return nil
}

func parseJSONArray(argsStr string) ([]string, error) {
	// Simple JSON array parser for strings
	argsStr = strings.TrimSpace(argsStr)
	if !strings.HasPrefix(argsStr, "[") || !strings.HasSuffix(argsStr, "]") {
		return nil, fmt.Errorf("not a valid JSON array")
	}

	// Remove brackets
	content := strings.Trim(argsStr, "[]")
	if content == "" {
		return []string{}, nil
	}

	// Split by comma, but be careful with quoted strings
	var args []string
	var current strings.Builder
	inQuotes := false
	quoteChar := byte(0)

	for i := 0; i < len(content); i++ {
		char := content[i]

		if !inQuotes && (char == '"' || char == '\'') {
			inQuotes = true
			quoteChar = char
		} else if inQuotes && char == quoteChar && (i == 0 || content[i-1] != '\\') {
			inQuotes = false
		} else if !inQuotes && char == ',' {
			arg := strings.TrimSpace(current.String())
			if arg != "" {
				// Remove surrounding quotes if present
				arg = strings.Trim(arg, `"'`)
				args = append(args, arg)
			}
			current.Reset()
		} else {
			current.WriteByte(char)
		}
	}

	// Add the last argument
	arg := strings.TrimSpace(current.String())
	if arg != "" {
		arg = strings.Trim(arg, `"'`)
		args = append(args, arg)
	}

	return args, nil
}

func containsImage(args []string, image string) bool {
	for _, arg := range args {
		if arg == image || strings.Contains(arg, image) {
			return true
		}
	}
	return false
}
