package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

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
		log.Fatal("Usage: go run combine_yaml.go <registry-dir>")
	}

	registryDir := os.Args[1]
	outputFile := "../config/comprehensive_mcp_servers.yaml"

	// Start with SUSE servers
	suseServers := []ServerConfig{
		{
			Name:  "uyuni",
			Image: "ghcr.io/uyuni-project/mcp-server-uyuni:latest",
			Type:  "server",
			Meta: map[string]interface{}{
				"category": "system-management",
				"tags":     []string{"uyuni", "patch-management", "linux", "system-administration", "suse"},
			},
			About: map[string]interface{}{
				"icon":  "https://apps.rancher.io/logos/suse-ai-deployer.png",
				"title": "SUSE Uyuni",
			},
			Source: map[string]interface{}{
				"commit":  "9635a2e04eddab77b31918acad8f49e23e5ee551",
				"project": "https://github.com/uyuni-project/mcp-server-uyuni",
			},
			Config: map[string]interface{}{
				"description": "Configure Uyuni server connection",
				"secrets": []map[string]interface{}{
					{"env": "UYUNI_SERVER", "example": "http://uyuni.example.com", "name": "uyuni.server"},
					{"env": "UYUNI_USER", "example": "admin", "name": "uyuni.user"},
					{"env": "UYUNI_PASS", "example": "your_password_here", "name": "uyuni.pass"},
					{"env": "UYUNI_MCP_SSL_VERIFY", "example": "true", "name": "uyuni.ssl_verify"},
					{"env": "UYUNI_MCP_WRITE_TOOLS_ENABLED", "example": "false", "name": "uyuni.write_tools_enabled"},
					{"env": "UYUNI_SSH_PRIV_KEY", "example": "-----BEGIN OPENSSH PRIVATE KEY-----\n...", "name": "uyuni.ssh_priv_key"},
					{"env": "UYUNI_SSH_PRIV_KEY_PASS", "example": "your_key_passphrase", "name": "uyuni.ssh_priv_key_pass"},
				},
			},
		},
		{
			Name:  "bugzilla",
			Image: "kskarthik/mcp-bugzilla:latest",
			Type:  "server",
			Meta: map[string]interface{}{
				"category": "issue-tracking",
				"tags":     []string{"bugzilla", "issue-tracking", "bug-reports", "suse"},
			},
			About: map[string]interface{}{
				"icon":  "https://apps.rancher.io/logos/suse-ai-deployer.png",
				"title": "SUSE Bugzilla",
			},
			Source: map[string]interface{}{
				"commit":  "040bc4b80f18e4a60deae1aa9f0dcf5c5b0bb0bf",
				"project": "https://github.com/openSUSE/mcp-bugzilla",
			},
			Config: map[string]interface{}{
				"description": "Configure Bugzilla server connection",
				"secrets": []map[string]interface{}{
					{"env": "BUGZILLA_SERVER", "example": "https://bugzilla.suse.com", "name": "bugzilla.server"},
					{"env": "BUGZILLA_APIKEY", "example": "your_api_key_here", "name": "bugzilla.apikey"},
				},
			},
		},
	}

	var allServers []ServerConfig
	allServers = append(allServers, suseServers...)

	// Process registry servers
	err := filepath.Walk(registryDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !strings.HasSuffix(path, ".yaml") || info.IsDir() {
			return nil
		}

		// Read and parse each YAML file
		data, err := ioutil.ReadFile(path)
		if err != nil {
			fmt.Printf("Warning: Failed to read %s: %v\n", path, err)
			return nil
		}

		var server ServerConfig
		if err := yaml.Unmarshal(data, &server); err != nil {
			fmt.Printf("Warning: Failed to parse %s: %v\n", path, err)
			return nil
		}

		// Skip archived servers
		if title, ok := server.About["title"].(string); ok && strings.Contains(title, "(Archived)") {
			fmt.Printf("Skipping archived server: %s\n", server.Name)
			return nil
		}

		allServers = append(allServers, server)
		return nil
	})

	if err != nil {
		log.Fatalf("Failed to walk registry directory: %v", err)
	}

	// Write combined YAML
	data, err := yaml.Marshal(allServers)
	if err != nil {
		log.Fatalf("Failed to marshal YAML: %v", err)
	}

	if err := ioutil.WriteFile(outputFile, data, 0644); err != nil {
		log.Fatalf("Failed to write output file: %v", err)
	}

	fmt.Printf("Successfully created %s with %d servers\n", outputFile, len(allServers))
}
