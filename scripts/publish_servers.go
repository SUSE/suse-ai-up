package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// Package represents a server package
type Package struct {
	RegistryType         string                `json:"registryType"`
	Identifier           string                `json:"identifier"`
	Transport            Transport             `json:"transport"`
	EnvironmentVariables []EnvironmentVariable `json:"environmentVariables,omitempty"`
}

// Transport defines how to connect to the server
type Transport struct {
	Type string `json:"type"`
}

// EnvironmentVariable represents an environment variable
type EnvironmentVariable struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Format      string `json:"format,omitempty"`
	IsSecret    bool   `json:"isSecret,omitempty"`
	Default     string `json:"default,omitempty"`
}

// Remote represents remote connection info
type Remote struct {
	Type    string   `json:"type"`
	URL     string   `json:"url"`
	Headers []Header `json:"headers,omitempty"`
}

// Header represents HTTP headers
type Header struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IsSecret    bool   `json:"isSecret,omitempty"`
	Value       string `json:"value,omitempty"`
}

// RegistryServerData represents the structure from found_servers.json
type RegistryServerData struct {
	Server struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Repository  *struct {
			URL    string `json:"url"`
			Source string `json:"source"`
		} `json:"repository,omitempty"`
		Version  string    `json:"version"`
		Packages []Package `json:"packages,omitempty"`
		Remotes  []Remote  `json:"remotes,omitempty"`
		Schema   string    `json:"$schema"`
	} `json:"server"`
	Meta map[string]interface{} `json:"_meta"`
}

// FoundServers represents the structure of found_servers.json
type FoundServers struct {
	Providers []string             `json:"providers"`
	Servers   []RegistryServerData `json:"servers"`
	Timestamp string               `json:"timestamp"`
}

// FoundDockerServers represents the structure of found_docker_servers.json
type FoundDockerServers struct {
	Source    string      `json:"source"`
	Servers   []MCPServer `json:"servers"`
	Timestamp string      `json:"timestamp"`
}

// MCPServer represents the local registry format
type MCPServer struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	Description      string                 `json:"description"`
	Repository       *Repository            `json:"repository,omitempty"`
	Version          string                 `json:"version,omitempty"`
	Packages         []Package              `json:"packages,omitempty"`
	ValidationStatus string                 `json:"validation_status"`
	DiscoveredAt     time.Time              `json:"discovered_at"`
	Tools            []MCPTool              `json:"tools,omitempty"`
	Meta             map[string]interface{} `json:"_meta,omitempty"`
}

// Repository represents repository information
type Repository struct {
	URL    string `json:"url"`
	Source string `json:"source"`
}

// MCPTool represents an MCP tool
type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

func main() {
	// Check command line args to determine which operation to run
	if len(os.Args) > 1 && os.Args[1] == "docker" {
		// Run Docker server publishing
		mainDocker()
		return
	}

	// Default: run official registry publishing
	mainPublish()
}

func mainPublish() {
	// Get file path from command line argument or default
	filePath := "found_servers.json"
	if len(os.Args) > 1 {
		filePath = os.Args[1]
	}

	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Error reading %s: %v", filePath, err)
	}

	// Try to parse as FoundServers first (official registry format)
	var found FoundServers
	var foundDocker FoundDockerServers
	var isDockerFormat bool

	if err := json.Unmarshal(data, &found); err != nil {
		// Try parsing as Docker format
		if err := json.Unmarshal(data, &foundDocker); err != nil {
			log.Fatalf("Error parsing %s: not a valid server format: %v", filePath, err)
		}
		isDockerFormat = true
		fmt.Printf("Detected Docker server format from source: %s\n", foundDocker.Source)
	}

	var serverCount int
	if isDockerFormat {
		serverCount = len(foundDocker.Servers)
	} else {
		serverCount = len(found.Servers)
	}

	fmt.Printf("Found %d servers to publish\n", serverCount)

	// Transform servers to local format
	var servers []*MCPServer

	if isDockerFormat {
		// Docker format - servers are already in MCPServer format
		for i := range foundDocker.Servers {
			servers = append(servers, &foundDocker.Servers[i])
		}
		fmt.Printf("Prepared %d Docker servers for publishing\n", len(servers))
	} else {
		// Official registry format - need transformation
		for i, regServer := range found.Servers {
			server := &MCPServer{
				ID:               fmt.Sprintf("registry-%d", i+1), // Generate simple ID
				Name:             regServer.Server.Name,
				Description:      regServer.Server.Description,
				Version:          regServer.Server.Version,
				Packages:         regServer.Server.Packages,
				ValidationStatus: "new",
				DiscoveredAt:     time.Now(),
				Tools:            []MCPTool{},
				Meta:             regServer.Meta,
			}

			// Copy repository if present
			if regServer.Server.Repository != nil {
				server.Repository = &Repository{
					URL:    regServer.Server.Repository.URL,
					Source: regServer.Server.Repository.Source,
				}
			}

			servers = append(servers, server)
		}
		fmt.Printf("Transformed %d official registry servers for publishing\n", len(servers))
	}

	// Send to bulk upload endpoint
	if err := uploadServers(servers); err != nil {
		log.Fatalf("Error uploading servers: %v", err)
	}

	fmt.Println("Successfully published servers to local registry!")
}

func mainDocker() {
	// Get file path from command line argument or default
	filePath := "found_docker_servers.json"
	if len(os.Args) > 2 {
		filePath = os.Args[2] // Second arg is the file path
	}

	// Read found_docker_servers.json
	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Error reading %s: %v", filePath, err)
	}

	var foundDocker FoundDockerServers
	if err := json.Unmarshal(data, &foundDocker); err != nil {
		log.Fatalf("Error parsing found_docker_servers.json: %v", err)
	}

	fmt.Printf("Found %d Docker servers to publish from source: %s\n", len(foundDocker.Servers), foundDocker.Source)

	// Convert to pointer slice for upload
	var servers []*MCPServer
	for i := range foundDocker.Servers {
		servers = append(servers, &foundDocker.Servers[i])
	}

	fmt.Printf("Prepared %d Docker servers for publishing\n", len(servers))

	// Send to bulk upload endpoint
	if err := uploadServers(servers); err != nil {
		log.Fatalf("Error uploading servers: %v", err)
	}

	fmt.Println("Successfully published Docker servers to local registry!")
}

func uploadServers(servers []*MCPServer) error {
	// Default to localhost:8911, but allow override via env var
	baseURL := os.Getenv("REGISTRY_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8911/api/v1"
	}

	url := baseURL + "/registry/upload/bulk"

	// Convert to JSON
	jsonData, err := json.Marshal(servers)
	if err != nil {
		return fmt.Errorf("error marshaling servers: %v", err)
	}

	// Create request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Send request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("Upload response: %s\n", string(body))
	return nil
}
