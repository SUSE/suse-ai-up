package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"gopkg.in/yaml.v3"
)

// ServerConfig represents the YAML structure
type ServerConfig struct {
	Name     string                   `yaml:"name"`
	Image    string                   `yaml:"image"`
	Type     string                   `yaml:"type"`
	Meta     map[string]interface{}   `yaml:"meta"`
	About    map[string]interface{}   `yaml:"about"`
	Source   map[string]interface{}   `yaml:"source"`
	Config   map[string]interface{}   `yaml:"config"`
	Packages []map[string]interface{} `yaml:"packages"`
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("Usage: go run upload_servers.go <yaml-file>")
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

	fmt.Printf("📊 Uploading %d servers to registry...\n", len(servers))

	// Upload servers to registry
	for _, server := range servers {
		if err := uploadServer(server); err != nil {
			fmt.Printf("❌ Failed to upload server %s: %v\n", server.Name, err)
		} else {
			fmt.Printf("✅ Uploaded server %s\n", server.Name)
		}
	}

	fmt.Printf("🎉 Finished uploading servers\n")
}

func uploadServer(server ServerConfig) error {
	// Convert to the format expected by the registry API
	registryServer := map[string]interface{}{
		"id":          server.Name,
		"name":        server.About["title"],
		"description": server.About["description"],
		"version":     "1.0.0",
		"repository": map[string]interface{}{
			"url": "",
		},
		"packages": server.Packages,
		"_meta":    server.Meta,
	}

	// Add packages if they exist
	if server.Packages != nil {
		registryServer["packages"] = server.Packages
	} else {
		// Create default stdio package
		registryServer["packages"] = []map[string]interface{}{
			{
				"registryType": "stdio",
				"identifier":   server.Image,
				"transport": map[string]interface{}{
					"type": "stdio",
				},
				"environmentVariables": []map[string]interface{}{},
			},
		}
	}

	jsonData, err := json.Marshal(registryServer)
	if err != nil {
		return fmt.Errorf("failed to marshal server: %w", err)
	}

	// Upload to registry
	url := "http://localhost:8913/api/v1/registry/upload"
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
