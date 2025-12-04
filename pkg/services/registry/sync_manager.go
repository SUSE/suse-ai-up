package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/models"
)

// SyncManager handles synchronization with external MCP registries
type SyncManager struct {
	store      clients.MCPServerStore
	httpClient *http.Client
}

// NewSyncManager creates a new sync manager
func NewSyncManager(store clients.MCPServerStore) *SyncManager {
	return &SyncManager{
		store: store,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SyncOfficialRegistry syncs servers from the official MCP registry
func (sm *SyncManager) SyncOfficialRegistry(ctx context.Context) error {
	log.Println("Starting sync with official MCP registry")

	// Official MCP registry URL
	url := "https://registry.modelcontextprotocol.io/index.json"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := sm.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch official registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("official registry returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var registryData struct {
		Servers []models.MCPServer `json:"servers"`
	}

	if err := json.Unmarshal(body, &registryData); err != nil {
		return fmt.Errorf("failed to parse registry data: %w", err)
	}

	// Process and store servers
	for _, server := range registryData.Servers {
		if server.Meta == nil {
			server.Meta = make(map[string]interface{})
		}
		server.Meta["source"] = "official-mcp"
		server.ValidationStatus = "approved" // Official registry servers are pre-validated
		server.DiscoveredAt = time.Now()

		// Generate ID if not present
		if server.ID == "" {
			server.ID = fmt.Sprintf("official-%s", strings.ToLower(strings.ReplaceAll(server.Name, " ", "-")))
		}

		if err := sm.store.CreateMCPServer(&server); err != nil {
			log.Printf("Failed to store official server %s: %v", server.ID, err)
			// Continue with other servers
		}
	}

	log.Printf("Successfully synced %d servers from official registry", len(registryData.Servers))
	return nil
}

// SyncDockerRegistry syncs servers from Docker Hub MCP namespace
func (sm *SyncManager) SyncDockerRegistry(ctx context.Context) error {
	log.Println("Starting sync with Docker MCP registry")

	// Docker Hub API for searching repositories in mcp namespace
	url := "https://index.docker.io/v1/search?q=mcp/&n=100"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := sm.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch Docker registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Docker registry returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var dockerData struct {
		Results []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			PullCount   int    `json:"pull_count"`
		} `json:"results"`
	}

	if err := json.Unmarshal(body, &dockerData); err != nil {
		return fmt.Errorf("failed to parse Docker registry data: %w", err)
	}

	serversCreated := 0
	for _, repo := range dockerData.Results {
		// Only process repositories in the mcp namespace
		if !strings.HasPrefix(repo.Name, "mcp/") {
			continue
		}

		server := models.MCPServer{
			ID:          fmt.Sprintf("docker-%s", strings.TrimPrefix(repo.Name, "mcp/")),
			Name:        repo.Name,
			Description: repo.Description,
			Version:     "latest",
			Packages: []models.Package{
				{
					RegistryType: "oci",
					Identifier:   repo.Name,
					Transport: models.Transport{
						Type: "stdio", // Most Docker MCP servers use stdio
					},
				},
			},
			ValidationStatus: "new",
			DiscoveredAt:     time.Now(),
			Meta: map[string]interface{}{
				"source":     "docker-mcp",
				"pull_count": repo.PullCount,
				"icon_url":   fmt.Sprintf("https://api.scout.docker.com/v1/policy/insights/org-image-score/badge/%s", repo.Name),
			},
		}

		if err := sm.store.CreateMCPServer(&server); err != nil {
			log.Printf("Failed to store Docker server %s: %v", server.ID, err)
			continue
		}
		serversCreated++
	}

	log.Printf("Successfully synced %d servers from Docker registry", serversCreated)
	return nil
}
