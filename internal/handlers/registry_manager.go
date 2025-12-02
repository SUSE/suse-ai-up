package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

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
			if server.Repository != nil {
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
		case "provider":
			// Check in repository source
			if server.Repository != nil && !strings.Contains(strings.ToLower(server.Repository.Source), strings.ToLower(fmt.Sprintf("%v", value))) {
				return false
			}
		case "source":
			// Check in meta.source for VirtualMCP entries
			if server.Meta == nil {
				return false
			}
			if source, ok := server.Meta["source"].(string); !ok || source != value {
				return false
			}
		}
	}

	return true
}

// SyncOfficialRegistry syncs all servers from the official MCP registry using pagination
func (rm *DefaultRegistryManager) SyncOfficialRegistry(ctx context.Context) (*SyncResult, error) {
	startTime := time.Now()
	result := &SyncResult{}

	log.Printf("Starting official registry sync")

	// Fetch all servers with pagination
	servers, err := rm.fetchAllOfficialServers(ctx, 100)
	if err != nil {
		result.Error = err.Error()
		result.Duration = time.Since(startTime)
		return result, fmt.Errorf("failed to fetch official registry: %w", err)
	}

	result.TotalFetched = len(servers)
	log.Printf("Fetched %d servers from official registry", len(servers))

	// Process servers: add new ones, update existing ones
	for _, server := range servers {
		existing, err := rm.store.GetMCPServer(server.ID)
		if err != nil && err.Error() != "server not found" {
			log.Printf("Error checking existing server %s: %v", server.ID, err)
			result.TotalErrors++
			continue
		}

		if existing == nil {
			// New server
			if err := rm.store.CreateMCPServer(server); err != nil {
				log.Printf("Error creating server %s: %v", server.ID, err)
				result.TotalErrors++
				continue
			}
			result.TotalAdded++
			log.Printf("Added new server: %s", server.ID)
		} else {
			// Update existing server
			if err := rm.store.UpdateMCPServer(server.ID, server); err != nil {
				log.Printf("Error updating server %s: %v", server.ID, err)
				result.TotalErrors++
				continue
			}
			result.TotalUpdated++
			log.Printf("Updated server: %s", server.ID)
		}
	}

	result.Duration = time.Since(startTime)
	log.Printf("Official registry sync completed in %v: %d fetched, %d added, %d updated, %d errors",
		result.Duration, result.TotalFetched, result.TotalAdded, result.TotalUpdated, result.TotalErrors)

	return result, nil
}

// fetchAllOfficialServers fetches all servers from the official MCP registry using pagination
func (rm *DefaultRegistryManager) fetchAllOfficialServers(ctx context.Context, limit int) ([]*models.MCPServer, error) {
	var allServers []*models.MCPServer
	cursor := ""
	pageCount := 0

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	for {
		// Build URL with pagination
		baseURL := "https://registry.modelcontextprotocol.io/v0.1/servers"
		params := url.Values{}
		params.Add("limit", fmt.Sprintf("%d", limit))
		if cursor != "" {
			params.Add("cursor", cursor)
		}

		fullURL := baseURL + "?" + params.Encode()

		// Create request with context
		req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		log.Printf("Fetching page %d from: %s", pageCount+1, fullURL)

		// Make request
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch page %d: %w", pageCount+1, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("API returned status %d for page %d", resp.StatusCode, pageCount+1)
		}

		// Parse response
		var response struct {
			Servers  []interface{} `json:"servers"`
			Metadata struct {
				NextCursor string `json:"nextCursor"`
				Count      int    `json:"count"`
			} `json:"metadata"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return nil, fmt.Errorf("failed to decode response for page %d: %w", pageCount+1, err)
		}

		log.Printf("Page %d: received %d servers, nextCursor: %s", pageCount+1, response.Metadata.Count, response.Metadata.NextCursor)

		// Convert servers
		convertedServers, err := rm.convertOfficialRegistryResponse(response.Servers)
		if err != nil {
			return nil, fmt.Errorf("failed to convert servers for page %d: %w", pageCount+1, err)
		}

		allServers = append(allServers, convertedServers...)
		pageCount++

		// Check if there are more pages
		if response.Metadata.NextCursor == "" {
			break
		}

		cursor = response.Metadata.NextCursor

		// Small delay between requests to be respectful to the API
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
			// Continue
		}
	}

	return allServers, nil
}

// convertOfficialRegistryResponse converts the official registry response format to our internal MCPServer format
func (rm *DefaultRegistryManager) convertOfficialRegistryResponse(servers []interface{}) ([]*models.MCPServer, error) {
	var converted []*models.MCPServer

	for _, serverEntry := range servers {
		serverMap, ok := serverEntry.(map[string]interface{})
		if !ok {
			log.Printf("Skipping invalid server entry: not a map")
			continue
		}

		serverData, ok := serverMap["server"].(map[string]interface{})
		if !ok {
			log.Printf("Skipping server entry without server data")
			continue
		}

		// Extract basic server information
		name, _ := serverData["name"].(string)
		description, _ := serverData["description"].(string)
		version, _ := serverData["version"].(string)

		if name == "" {
			log.Printf("Skipping server without name")
			continue
		}

		// Create server ID from name
		serverID := strings.ReplaceAll(name, "/", "_")

		server := &models.MCPServer{
			ID:          serverID,
			Name:        name,
			Description: description,
			Version:     version,
		}

		// Extract repository information
		if repoData, ok := serverData["repository"].(map[string]interface{}); ok {
			repo := &models.Repository{}
			if repoURL, ok := repoData["url"].(string); ok {
				repo.URL = repoURL
			}
			if source, ok := repoData["source"].(string); ok {
				repo.Source = source
			}
			server.Repository = repo
		}

		// Extract packages (OCI, NPM, etc.)
		if packagesData, ok := serverData["packages"].([]interface{}); ok {
			var packages []models.Package
			for _, pkgData := range packagesData {
				if pkgMap, ok := pkgData.(map[string]interface{}); ok {
					pkg := models.Package{}
					if regType, ok := pkgMap["registryType"].(string); ok {
						pkg.RegistryType = regType
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
					packages = append(packages, pkg)
				}
			}
			server.Packages = packages
		}

		// Extract metadata
		if metaData, ok := serverMap["_meta"].(map[string]interface{}); ok {
			server.Meta = metaData
		}

		converted = append(converted, server)
	}

	return converted, nil
}
