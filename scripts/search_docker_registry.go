package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"suse-ai-up/pkg/models"
)

// DockerHubRepository represents a repository from Docker Hub API
type DockerHubRepository struct {
	Name              string   `json:"name"`
	Namespace         string   `json:"namespace"`
	RepositoryType    string   `json:"repository_type"`
	Status            int      `json:"status"`
	StatusDescription string   `json:"status_description"`
	Description       string   `json:"description"`
	IsPrivate         bool     `json:"is_private"`
	StarCount         int      `json:"star_count"`
	PullCount         int      `json:"pull_count"`
	LastUpdated       string   `json:"last_updated"`
	LastModified      string   `json:"last_modified"`
	DateRegistered    string   `json:"date_registered"`
	Affiliation       string   `json:"affiliation"`
	MediaTypes        []string `json:"media_types"`
	ContentTypes      []string `json:"content_types"`
	Categories        []struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	} `json:"categories"`
	StorageSize int64  `json:"storage_size"`
	Source      string `json:"source"`
}

// DockerHubResponse represents the response from Docker Hub API
type DockerHubResponse struct {
	Count    int                   `json:"count"`
	Next     string                `json:"next"`
	Previous string                `json:"previous"`
	Results  []DockerHubRepository `json:"results"`
}

// DockerServer represents a Docker MCP server entry
type DockerServer struct {
	Name        string `json:"name"`
	Description string `json:"description"`
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
	Packages         []models.Package       `json:"packages,omitempty"`
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

func main() {
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
		fmt.Printf("Processed: %s\n", server.Name)
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
	var allServers []DockerServer

	// Docker Hub API pagination
	url := "https://hub.docker.com/v2/repositories/mcp/?page_size=100"

	for url != "" {
		resp, err := http.Get(url)
		if err != nil {
			return nil, fmt.Errorf("error fetching Docker Hub repositories: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("Docker Hub API returned status %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error reading response: %v", err)
		}

		var hubResponse DockerHubResponse
		if err := json.Unmarshal(body, &hubResponse); err != nil {
			return nil, fmt.Errorf("error parsing Docker Hub response: %v", err)
		}

		// Convert Docker Hub repositories to our server format
		for _, repo := range hubResponse.Results {
			server := DockerServer{
				Name:        repo.Name,
				Description: repo.Description,
			}
			allServers = append(allServers, server)
		}

		// Get next page URL
		url = hubResponse.Next
	}

	return allServers, nil
}

func transformToDockerMCPServer(dockerServer DockerServer, index int) *DockerMCPServer {
	server := &DockerMCPServer{
		ID:               fmt.Sprintf("docker-%d", index),
		Name:             dockerServer.Name,
		Description:      dockerServer.Description,
		Version:          "latest", // Docker Hub doesn't provide version info in this API
		ValidationStatus: "new",
		DiscoveredAt:     time.Now(),
		Tools:            parseToolsFromDescription(dockerServer.Description),
		Meta: map[string]interface{}{
			"source": "docker-mcp",
		},
	}

	// Set repository to Docker Hub URL
	server.Repository = &DockerRepository{
		URL:    fmt.Sprintf("https://hub.docker.com/r/mcp/%s", dockerServer.Name),
		Source: "dockerhub",
	}

	// Create Docker package
	server.Packages = []models.Package{
		{
			RegistryType: "oci",
			Identifier:   fmt.Sprintf("mcp/%s", dockerServer.Name),
			Transport: models.Transport{
				Type: "stdio",
			},
		},
	}

	return server
}

func parseToolsFromDescription(description string) []DockerMCPTool {
	// Docker Hub descriptions are much shorter, so we'll create a generic tool
	// based on the server description. Most MCP servers provide basic functionality.
	var tools []DockerMCPTool

	// For now, create a single generic tool based on the description
	// This is a simplified approach since Docker Hub doesn't provide detailed tool info
	if description != "" && len(description) > 10 {
		// Create a basic tool with the server description
		inputSchema := map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}

		tools = append(tools, DockerMCPTool{
			Name:        "execute",
			Description: description,
			InputSchema: inputSchema,
		})
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
