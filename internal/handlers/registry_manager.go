package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"suse-ai-up/pkg/models"
)

// DefaultRegistryManager implements RegistryManagerInterface
type DefaultRegistryManager struct {
	store MCPServerStore
}

// NewDefaultRegistryManager creates a new default registry manager
func NewDefaultRegistryManager(store MCPServerStore) *DefaultRegistryManager {
	return &DefaultRegistryManager{
		store: store,
	}
}

// UploadRegistryEntries uploads multiple registry entries
func (rm *DefaultRegistryManager) UploadRegistryEntries(entries []*models.MCPServer) error {
	for _, server := range entries {
		if err := rm.store.CreateMCPServer(server); err != nil {
			return fmt.Errorf("failed to create server %s: %w", server.ID, err)
		}
	}
	return nil
}

// Clear removes all MCP servers from the registry
func (rm *DefaultRegistryManager) Clear() error {
	servers := rm.store.ListMCPServers()
	for _, server := range servers {
		if err := rm.store.DeleteMCPServer(server.ID); err != nil {
			return fmt.Errorf("failed to delete server %s: %w", server.ID, err)
		}
	}
	return nil
}

// LoadFromCustomSource loads registry entries from a custom source URL
func (rm *DefaultRegistryManager) LoadFromCustomSource(sourceURL string) error {
	resp, err := http.Get(sourceURL)
	if err != nil {
		return fmt.Errorf("failed to fetch from source %s: %w", sourceURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("source returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	var servers []*models.MCPServer
	if err := json.Unmarshal(body, &servers); err != nil {
		return fmt.Errorf("failed to unmarshal servers: %w", err)
	}

	return rm.UploadRegistryEntries(servers)
}

// SearchServers searches for servers with query and filters
func (rm *DefaultRegistryManager) SearchServers(query string, filters map[string]interface{}) ([]*models.MCPServer, error) {
	allServers := rm.store.ListMCPServers()

	var results []*models.MCPServer

	for _, server := range allServers {
		// Apply text search
		if query != "" {
			searchText := strings.ToLower(query)
			serverText := strings.ToLower(fmt.Sprintf("%s %s", server.Name, server.Description))
			if server.Repository.Source != "" {
				serverText += " " + strings.ToLower(server.Repository.Source)
			}
			if !strings.Contains(serverText, searchText) {
				continue
			}
		}

		// Apply filters
		if !rm.matchesFilters(server, filters) {
			continue
		}

		results = append(results, server)
	}

	return results, nil
}

// matchesFilters checks if a server matches the provided filters
func (rm *DefaultRegistryManager) matchesFilters(server *models.MCPServer, filters map[string]interface{}) bool {
	if filters == nil {
		return true
	}

	for key, value := range filters {
		switch key {
		case "transport":
			// Check in packages for transport type
			if len(server.Packages) > 0 {
				found := false
				for _, pkg := range server.Packages {
					if pkg.Transport.Type == value {
						found = true
						break
					}
				}
				if !found {
					return false
				}
			}
		case "registryType":
			// Check in packages for registry type
			if len(server.Packages) > 0 {
				found := false
				for _, pkg := range server.Packages {
					if pkg.RegistryType == value {
						found = true
						break
					}
				}
				if !found {
					return false
				}
			}
		case "validationStatus":
			if server.ValidationStatus != value {
				return false
			}
		case "source":
			// Check in meta.source for YAML source info (git repo, etc.)
			if server.Meta == nil {
				return false
			}
			if source, ok := server.Meta["source"].(string); !ok || source != value {
				return false
			}
		case "registry_source":
			// Check registry source (yaml, official, docker, etc.)
			if server.Meta == nil {
				return false
			}
			if registrySource, ok := server.Meta["registry_source"].(string); !ok || registrySource != value {
				return false
			}
		}
	}

	return true
}
