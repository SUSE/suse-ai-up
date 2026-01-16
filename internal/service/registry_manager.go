package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"suse-ai-up/pkg/models"
)

// MCPServerStore interface for MCP server storage operations
type MCPServerStore interface {
	CreateMCPServer(server *models.MCPServer) error
	GetMCPServer(id string) (*models.MCPServer, error)
	UpdateMCPServer(id string, updated *models.MCPServer) error
	DeleteMCPServer(id string) error
	ListMCPServers() []*models.MCPServer
}

// RegistryManager handles MCP registry synchronization and management
type RegistryManager struct {
	store          MCPServerStore
	httpClient     *http.Client
	enableOfficial bool
	syncInterval   time.Duration
	customSources  []string
	lastSync       time.Time
}

// NewRegistryManager creates a new registry manager
func NewRegistryManager(store MCPServerStore, enableOfficial bool, syncInterval time.Duration, customSources []string) *RegistryManager {
	return &RegistryManager{
		store:          store,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		enableOfficial: enableOfficial,
		syncInterval:   syncInterval,
		customSources:  customSources,
	}
}

// SyncOfficialRegistry syncs from the official MCP registry
func (rm *RegistryManager) SyncOfficialRegistry(ctx context.Context) error {
	if !rm.enableOfficial {
		log.Printf("RegistryManager: Official registry sync disabled")
		return nil
	}

	// Check if we need to sync based on interval
	if time.Since(rm.lastSync) < rm.syncInterval {
		log.Printf("RegistryManager: Skipping sync, last sync was %v ago", time.Since(rm.lastSync))
		return nil
	}

	log.Printf("RegistryManager: Starting official registry sync")

	// Fetch from official registry
	servers, err := rm.fetchOfficialRegistry(ctx)
	if err != nil {
		log.Printf("RegistryManager: Failed to fetch official registry: %v", err)
		return fmt.Errorf("failed to fetch official registry: %w", err)
	}

	// Store servers
	for _, server := range servers {
		server.ValidationStatus = "synced"
		server.DiscoveredAt = time.Now()

		if err := rm.store.CreateMCPServer(server); err != nil {
			// Log error but continue with other servers
			log.Printf("RegistryManager: Failed to store server %s: %v", server.ID, err)
		}
	}

	rm.lastSync = time.Now()
	log.Printf("RegistryManager: Successfully synced %d servers from official registry", len(servers))

	return nil
}

// fetchOfficialRegistry fetches servers from the official MCP registry
func (rm *RegistryManager) fetchOfficialRegistry(ctx context.Context) ([]*models.MCPServer, error) {
	const officialURL = "https://registry.modelcontextprotocol.io/v0/servers?limit=100"

	req, err := http.NewRequestWithContext(ctx, "GET", officialURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := rm.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	var result struct {
		Servers []map[string]interface{} `json:"servers"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var servers []*models.MCPServer
	for _, serverData := range result.Servers {
		server, err := rm.parseOfficialServer(serverData)
		if err != nil {
			log.Printf("RegistryManager: Failed to parse server: %v", err)
			continue
		}
		servers = append(servers, server)
	}

	return servers, nil
}

// parseOfficialServer converts official registry format to our MCPServer model
func (rm *RegistryManager) parseOfficialServer(data map[string]interface{}) (*models.MCPServer, error) {
	serverData, ok := data["server"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid server data format")
	}

	server := &models.MCPServer{}
	if meta, ok := data["_meta"].(map[string]interface{}); ok {
		server.Meta = meta
	}

	// Extract basic fields
	if id, ok := serverData["name"].(string); ok {
		server.ID = id
		server.Name = id
	}

	if desc, ok := serverData["description"].(string); ok {
		server.Description = desc
	}

	if version, ok := serverData["version"].(string); ok {
		server.Version = version
	}

	// Extract repository
	if repoData, ok := serverData["repository"].(map[string]interface{}); ok {
		repo := models.Repository{}
		if url, ok := repoData["url"].(string); ok {
			repo.URL = url
		}
		if source, ok := repoData["source"].(string); ok {
			repo.Source = source
		}
		server.Repository = repo
	}

	// Extract packages
	if packagesData, ok := serverData["packages"].([]interface{}); ok {
		var packages []models.Package
		for _, pkgData := range packagesData {
			if pkgMap, ok := pkgData.(map[string]interface{}); ok {
				pkg := models.Package{}

				if registryType, ok := pkgMap["registryType"].(string); ok {
					pkg.RegistryType = registryType
				}

				if identifier, ok := pkgMap["identifier"].(string); ok {
					pkg.Identifier = identifier
				}

				if transportData, ok := pkgMap["transport"].(map[string]interface{}); ok {
					transport := models.Transport{}
					if transportType, ok := transportData["type"].(string); ok {
						transport.Type = transportType
					}
					pkg.Transport = transport
				}

				if envVars, ok := pkgMap["environmentVariables"].([]interface{}); ok {
					var envs []models.EnvironmentVariable
					for _, envData := range envVars {
						if envMap, ok := envData.(map[string]interface{}); ok {
							env := models.EnvironmentVariable{}
							if name, ok := envMap["name"].(string); ok {
								env.Name = name
							}
							if desc, ok := envMap["description"].(string); ok {
								env.Description = desc
							}
							if isSecret, ok := envMap["isSecret"].(bool); ok {
								env.IsSecret = isSecret
							}
							envs = append(envs, env)
						}
					}
					pkg.EnvironmentVariables = envs
				}

				packages = append(packages, pkg)
			}
		}
		server.Packages = packages
	}

	return server, nil
}

// UploadRegistryEntries uploads custom registry entries
func (rm *RegistryManager) UploadRegistryEntries(entries []*models.MCPServer) error {
	log.Printf("RegistryManager: Uploading %d registry entries", len(entries))

	for _, entry := range entries {
		// Set validation status for uploaded entries
		entry.ValidationStatus = "uploaded"
		entry.DiscoveredAt = time.Now()

		if err := rm.store.CreateMCPServer(entry); err != nil {
			log.Printf("RegistryManager: Failed to store uploaded server %s: %v", entry.ID, err)
			// Continue with other entries
		}
	}

	log.Printf("RegistryManager: Successfully uploaded registry entries")
	return nil
}

// LoadFromCustomSource loads registry data from a custom source
func (rm *RegistryManager) LoadFromCustomSource(sourceURL string) error {
	log.Printf("RegistryManager: Loading from custom source: %s", sourceURL)

	u, err := url.Parse(sourceURL)
	if err != nil {
		return fmt.Errorf("invalid source URL: %w", err)
	}

	var data []byte
	switch u.Scheme {
	case "file":
		data, err = rm.loadFromFile(u.Path)
	case "http", "https":
		data, err = rm.loadFromHTTP(sourceURL)
	default:
		return fmt.Errorf("unsupported source scheme: %s", u.Scheme)
	}

	if err != nil {
		return fmt.Errorf("failed to load from source: %w", err)
	}

	// Try to parse as JSON first, then YAML
	var entries []*models.MCPServer

	// Try JSON
	if err := json.Unmarshal(data, &entries); err != nil {
		// If JSON fails, try parsing as {servers: [...]} format
		var wrapper struct {
			Servers []*models.MCPServer `json:"servers"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return fmt.Errorf("failed to parse registry data: %w", err)
		}
		entries = wrapper.Servers
	}

	return rm.UploadRegistryEntries(entries)
}

// loadFromFile loads data from a local file
func (rm *RegistryManager) loadFromFile(filePath string) ([]byte, error) {
	// Handle relative paths
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(".", filePath)
	}

	return os.ReadFile(filePath)
}

// loadFromHTTP loads data from an HTTP URL
func (rm *RegistryManager) loadFromHTTP(url string) ([]byte, error) {
	resp, err := rm.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP request failed with status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// SyncAllSources syncs from all configured sources
func (rm *RegistryManager) SyncAllSources(ctx context.Context) error {
	log.Printf("RegistryManager: Starting sync from all sources")

	// Sync official registry if enabled
	if err := rm.SyncOfficialRegistry(ctx); err != nil {
		log.Printf("RegistryManager: Official registry sync failed: %v", err)
	}

	// Sync custom sources
	for _, source := range rm.customSources {
		if err := rm.LoadFromCustomSource(source); err != nil {
			log.Printf("RegistryManager: Failed to sync custom source %s: %v", source, err)
		}
	}

	log.Printf("RegistryManager: Completed sync from all sources")
	return nil
}

// SearchServers searches for servers matching the given criteria
func (rm *RegistryManager) SearchServers(query string, filters map[string]interface{}) ([]*models.MCPServer, error) {
	allServers := rm.store.ListMCPServers()
	var results []*models.MCPServer

	for _, server := range allServers {
		// Apply filters
		if rm.matchesFilters(server, query, filters) {
			results = append(results, server)
		}
	}

	return results, nil
}

// matchesFilters checks if a server matches the search criteria
func (rm *RegistryManager) matchesFilters(server *models.MCPServer, query string, filters map[string]interface{}) bool {
	// Text search in name and description
	if query != "" {
		queryLower := strings.ToLower(query)
		if !strings.Contains(strings.ToLower(server.Name), queryLower) &&
			!strings.Contains(strings.ToLower(server.Description), queryLower) {
			return false
		}
	}

	// Apply additional filters
	for key, value := range filters {
		switch key {
		case "transport":
			if transportType, ok := value.(string); ok {
				found := false
				for _, pkg := range server.Packages {
					if pkg.Transport.Type == transportType {
						found = true
						break
					}
				}
				if !found {
					return false
				}
			}
		case "registryType":
			if registryType, ok := value.(string); ok {
				found := false
				for _, pkg := range server.Packages {
					if pkg.RegistryType == registryType {
						found = true
						break
					}
				}
				if !found {
					return false
				}
			}
		case "validationStatus":
			if status, ok := value.(string); ok && server.ValidationStatus != status {
				return false
			}
		}
	}

	return true
}
