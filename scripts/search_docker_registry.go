package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// DockerServerMetadata represents metadata extracted from Docker MCP server
type DockerServerMetadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Homepage    string `json:"homepage"`
	Repository  string `json:"repository"`
	Readme      string `json:"readme"`
}

// GitHubFile represents a file from GitHub API
type GitHubFile struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	DownloadURL string `json:"download_url"`
}

// DockerServer represents a Docker MCP server entry
type DockerServer struct {
	Name     string               `json:"name"`
	Metadata DockerServerMetadata `json:"metadata"`
}

// DockerRegistryResponse represents the response from Docker registry API
type DockerRegistryResponse struct {
	Servers []DockerServer `json:"servers"`
}

// DockerMCPServer represents the local registry format for Docker servers
type DockerMCPServer struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	Description      string                 `json:"description"`
	Repository       *DockerRepository      `json:"repository,omitempty"`
	Version          string                 `json:"version,omitempty"`
	Packages         []Package              `json:"packages,omitempty"`
	ValidationStatus string                 `json:"validation_status"`
	DiscoveredAt     time.Time              `json:"discovered_at"`
	Tools            []DockerMCPTool        `json:"tools,omitempty"`
	Meta             map[string]interface{} `json:"_meta,omitempty"`
}

// DockerRepository represents repository information
type DockerRepository struct {
	URL    string `json:"url"`
	Source string `json:"source"`
}

// DockerMCPTool represents an MCP tool
type DockerMCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// ToolInfo represents parsed tool information from README
type ToolInfo struct {
	Name        string
	Description string
	Parameters  map[string]interface{}
}

func mainSearch() {
	fmt.Println("Fetching Docker MCP registry servers...")

	// Get list of servers from GitHub API
	servers, err := fetchDockerServers()
	if err != nil {
		log.Fatalf("Error fetching Docker servers: %v", err)
	}

	fmt.Printf("Found %d Docker MCP servers\n", len(servers))

	// Transform to local format
	var mcpservers []*DockerMCPServer
	for i, server := range servers {
		mcpServer := transformToDockerMCPServer(server, i+1)
		mcpservers = append(mcpservers, mcpServer)
		fmt.Printf("Processed: %s (%s)\n", server.Name, server.Metadata.Version)
	}

	// Save to JSON file
	output := map[string]interface{}{
		"source":    "docker-mcp",
		"servers":   mcpservers,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	outputJSON, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		log.Fatalf("Error marshaling output: %v", err)
	}

	err = writeFile("../found_docker_servers.json", outputJSON)
	if err != nil {
		log.Fatalf("Error writing output file: %v", err)
	}

	fmt.Printf("\n=== DOCKER REGISTRY SEARCH COMPLETE ===\n")
	fmt.Printf("Total servers processed: %d\n", len(mcpservers))
	fmt.Printf("Results saved to found_docker_servers.json\n")
}

func fetchDockerServers() ([]DockerServer, error) {
	// Fetch list of server directories from GitHub API
	url := "https://api.github.com/repos/modelcontextprotocol/servers/contents/src"
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("error fetching server list: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %v", err)
	}

	var files []GitHubFile
	if err := json.Unmarshal(body, &files); err != nil {
		return nil, fmt.Errorf("error parsing GitHub response: %v", err)
	}

	var servers []DockerServer
	for _, file := range files {
		if file.Type == "dir" {
			server, err := fetchServerMetadata(file.Name)
			if err != nil {
				fmt.Printf("Warning: Failed to fetch metadata for %s: %v\n", file.Name, err)
				continue
			}
			servers = append(servers, *server)
		}
	}

	return servers, nil
}

func fetchServerMetadata(serverName string) (*DockerServer, error) {
	// Fetch package.json
	packageURL := fmt.Sprintf("https://raw.githubusercontent.com/modelcontextprotocol/servers/main/src/%s/package.json", serverName)
	packageResp, err := http.Get(packageURL)
	if err != nil {
		return nil, fmt.Errorf("error fetching package.json: %v", err)
	}
	defer packageResp.Body.Close()

	if packageResp.StatusCode != 200 {
		return nil, fmt.Errorf("package.json not found (status %d)", packageResp.StatusCode)
	}

	packageBody, err := io.ReadAll(packageResp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading package.json: %v", err)
	}

	var metadata DockerServerMetadata
	if err := json.Unmarshal(packageBody, &metadata); err != nil {
		return nil, fmt.Errorf("error parsing package.json: %v", err)
	}

	// Fetch README.md
	readmeURL := fmt.Sprintf("https://raw.githubusercontent.com/modelcontextprotocol/servers/main/src/%s/README.md", serverName)
	readmeResp, err := http.Get(readmeURL)
	if err != nil {
		return nil, fmt.Errorf("error fetching README.md: %v", err)
	}
	defer readmeResp.Body.Close()

	if readmeResp.StatusCode == 200 {
		readmeBody, err := io.ReadAll(readmeResp.Body)
		if err == nil {
			metadata.Readme = string(readmeBody)
		}
	}

	// Set repository URL
	metadata.Repository = fmt.Sprintf("https://github.com/modelcontextprotocol/servers/tree/main/src/%s", serverName)

	return &DockerServer{
		Name:     serverName,
		Metadata: metadata,
	}, nil
}

func transformToDockerMCPServer(dockerServer DockerServer, index int) *DockerMCPServer {
	server := &DockerMCPServer{
		ID:               fmt.Sprintf("docker-%d", index),
		Name:             dockerServer.Metadata.Name,
		Description:      dockerServer.Metadata.Description,
		Version:          dockerServer.Metadata.Version,
		ValidationStatus: "new",
		DiscoveredAt:     time.Now(),
		Tools:            parseToolsFromReadme(dockerServer.Metadata.Readme),
		Meta: map[string]interface{}{
			"source": "docker-mcp",
		},
	}

	// Set repository
	if dockerServer.Metadata.Repository != "" {
		server.Repository = &DockerRepository{
			URL:    dockerServer.Metadata.Repository,
			Source: "github",
		}
	}

	// Create Docker package
	server.Packages = []Package{
		{
			RegistryType: "oci",
			Identifier:   fmt.Sprintf("mcp/%s", dockerServer.Name),
			Transport: Transport{
				Type: "stdio",
			},
		},
	}

	return server
}

func parseToolsFromReadme(readme string) []DockerMCPTool {
	var tools []DockerMCPTool

	// Simple regex to find tool sections in README
	// Look for lines like: - **toolName** description
	toolPattern := regexp.MustCompile(`- \*\*(\w+)\*\*\s*(.*?)(?:\n|$)`)
	matches := toolPattern.FindAllStringSubmatch(readme, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			toolName := match[1]
			toolDesc := strings.TrimSpace(match[2])

			// Skip if description is too short or contains "Inputs:" (not a tool description)
			if len(toolDesc) < 10 || strings.Contains(toolDesc, "Inputs:") {
				continue
			}

			// Create basic input schema (could be enhanced)
			inputSchema := map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			}

			tools = append(tools, DockerMCPTool{
				Name:        toolName,
				Description: toolDesc,
				InputSchema: inputSchema,
			})
		}
	}

	return tools
}

func writeFile(filename string, data []byte) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(data)
	return err
}
