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

	// Try multiple possible URLs for MCP registries
	urls := []string{
		"https://mcpservers.org/remote-mcp-servers.json",
		"https://raw.githubusercontent.com/modelcontextprotocol/registry/main/index.json",
		"https://raw.githubusercontent.com/modelcontextprotocol/examples/main/README.md",
	}

	// Also add some well-known MCP servers directly
	wellKnownServers := []models.MCPServer{
		{
			ID:          "filesystem",
			Name:        "File System",
			Description: "MCP server for file system operations",
			Version:     "1.0.0",
			Packages: []models.Package{
				{
					RegistryType: "npm",
					Identifier:   "@modelcontextprotocol/server-filesystem",
					Transport: models.Transport{
						Type: "stdio",
					},
				},
			},
			RouteAssignments: []models.RouteAssignment{
				{
					ID:          "filesystem-default",
					UserIDs:     []string{}, // No specific users
					GroupIDs:    []string{"mcp-users"},
					AutoSpawn:   true,
					Permissions: "read",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				},
			},
			AutoSpawn: &models.AutoSpawnConfig{
				Enabled:        true,
				ConnectionType: models.ConnectionTypeLocalStdio,
				Command:        "npx",
				Args:           []string{"@modelcontextprotocol/server-filesystem", "--help"},
			},
			ValidationStatus: "approved",
			Meta: map[string]interface{}{
				"source":   "well-known",
				"category": "utility",
				"tags":     []string{"filesystem", "files", "directories"},
			},
		},
		{
			ID:          "git",
			Name:        "Git",
			Description: "MCP server for Git repository operations",
			Version:     "1.0.0",
			Packages: []models.Package{
				{
					RegistryType: "npm",
					Identifier:   "@modelcontextprotocol/server-git",
					Transport: models.Transport{
						Type: "stdio",
					},
				},
			},
			ValidationStatus: "approved",
			Meta: map[string]interface{}{
				"source":   "well-known",
				"category": "development",
				"tags":     []string{"git", "version-control", "repository"},
			},
		},
		{
			ID:          "sqlite",
			Name:        "SQLite",
			Description: "MCP server for SQLite database operations",
			Version:     "1.0.0",
			Packages: []models.Package{
				{
					RegistryType: "npm",
					Identifier:   "@modelcontextprotocol/server-sqlite",
					Transport: models.Transport{
						Type: "stdio",
					},
				},
			},
			ValidationStatus: "approved",
			Meta: map[string]interface{}{
				"source":   "well-known",
				"category": "database",
				"tags":     []string{"sqlite", "database", "sql"},
			},
		},
		{
			ID:          "everything",
			Name:        "Everything",
			Description: "MCP server providing access to a variety of tools and resources",
			Version:     "1.0.0",
			Packages: []models.Package{
				{
					RegistryType: "npm",
					Identifier:   "@modelcontextprotocol/server-everything",
					Transport: models.Transport{
						Type: "stdio",
					},
				},
			},
			ValidationStatus: "approved",
			Meta: map[string]interface{}{
				"source":   "well-known",
				"category": "utility",
				"tags":     []string{"everything", "comprehensive", "tools"},
			},
		},
	}

	var lastErr error
	totalSynced := 0

	for _, url := range urls {
		log.Printf("Trying to sync from: %s", url)

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			lastErr = fmt.Errorf("failed to create request for %s: %w", url, err)
			continue
		}

		resp, err := sm.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to fetch from %s: %w", url, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("%s returned status %d", url, resp.StatusCode)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("failed to read response from %s: %w", url, err)
			continue
		}

		// Try to parse as direct array of servers first
		var servers []models.MCPServer
		if err := json.Unmarshal(body, &servers); err != nil {
			// Try parsing as object with servers field
			var registryData struct {
				Servers []models.MCPServer `json:"servers"`
			}
			if err := json.Unmarshal(body, &registryData); err != nil {
				// Try parsing as object with different field names
				var altRegistryData struct {
					Data []models.MCPServer `json:"data"`
				}
				if err := json.Unmarshal(body, &altRegistryData); err != nil {
					lastErr = fmt.Errorf("failed to parse data from %s: %w", url, err)
					continue
				}
				servers = altRegistryData.Data
			} else {
				servers = registryData.Servers
			}
		}

		// Process and store servers
		syncedFromURL := 0
		for _, server := range servers {
			if server.Meta == nil {
				server.Meta = make(map[string]interface{})
			}
			server.Meta["source"] = url
			server.ValidationStatus = "approved" // Remote registries are pre-validated
			server.DiscoveredAt = time.Now()

			// Generate ID if not present
			if server.ID == "" {
				server.ID = fmt.Sprintf("remote-%s", strings.ToLower(strings.ReplaceAll(server.Name, " ", "-")))
			}

			if err := sm.store.CreateMCPServer(&server); err != nil {
				log.Printf("Failed to store server %s: %v", server.ID, err)
				// Continue with other servers
			} else {
				syncedFromURL++
			}
		}

		log.Printf("Successfully synced %d servers from %s", syncedFromURL, url)
		totalSynced += syncedFromURL
	}

	// If no servers were synced from remote sources, add well-known servers
	if totalSynced == 0 {
		log.Println("No servers synced from remote registries, adding well-known MCP servers")
		for _, server := range wellKnownServers {
			server.DiscoveredAt = time.Now()
			if server.ID == "" {
				server.ID = fmt.Sprintf("wellknown-%s", strings.ToLower(strings.ReplaceAll(server.Name, " ", "-")))
			}

			if err := sm.store.CreateMCPServer(&server); err != nil {
				log.Printf("Failed to store well-known server %s: %v", server.ID, err)
			} else {
				totalSynced++
			}
		}
	}

	if totalSynced == 0 {
		return fmt.Errorf("failed to sync from any registry or add well-known servers, last error: %w", lastErr)
	}

	log.Printf("Successfully synced %d servers total from remote registries and well-known sources", totalSynced)
	return nil
}


