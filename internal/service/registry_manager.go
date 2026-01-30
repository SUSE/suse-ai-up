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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1" // New import
	"k8s.io/client-go/kubernetes" // New import
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
	customSources  []models.RegistrySourceConfig // Changed type
	lastSync       time.Time
	k8sClient      kubernetes.Interface        // New field
}

// NewRegistryManager creates a new registry manager
func NewRegistryManager(store MCPServerStore, enableOfficial bool, syncInterval time.Duration, customSources []models.RegistrySourceConfig, k8sClient kubernetes.Interface) *RegistryManager {
	return &RegistryManager{
		store:          store,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		enableOfficial: enableOfficial,
		syncInterval:   syncInterval,
		customSources:  customSources,
		k8sClient:      k8sClient, // Assign the new client
	}
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
func (rm *RegistryManager) LoadFromCustomSource(sourceConfig models.RegistrySourceConfig) error {
	log.Printf("RegistryManager: Loading from custom source: %s", sourceConfig.URL)

	u, err := url.Parse(sourceConfig.URL)
	if err != nil {
		return fmt.Errorf("invalid source URL: %w", err)
	}

	var data []byte
	var authToken string // New variable for token

	// Fetch auth token if configured
	if sourceConfig.Auth != nil && sourceConfig.Auth.SecretName != "" && sourceConfig.Auth.SecretKey != "" {
		if rm.k8sClient == nil {
			return fmt.Errorf("kubernetes client is not configured to fetch auth token for source: %s", sourceConfig.URL)
		}

		// Assume namespace is where the pod is running, usually 'default' or from env var
		namespace := os.Getenv("POD_NAMESPACE")
		if namespace == "" {
			namespace = "default"
		}

		secret, err := rm.k8sClient.CoreV1().Secrets(namespace).Get(context.Background(), sourceConfig.Auth.SecretName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get secret %s/%s for registry source %s: %w", namespace, sourceConfig.Auth.SecretName, sourceConfig.URL, err)
		}

		tokenBytes, ok := secret.Data[sourceConfig.Auth.SecretKey]
		if !ok {
			return fmt.Errorf("secret %s/%s does not contain key %s for registry source %s", namespace, sourceConfig.Auth.SecretName, sourceConfig.Auth.SecretKey, sourceConfig.URL)
		}
		authToken = string(tokenBytes)
	}

	switch u.Scheme {
	case "file":
		data, err = rm.loadFromFile(u.Path)
	case "http", "https":
		// Pass the token to loadFromHTTP
		data, err = rm.loadFromHTTP(sourceConfig.URL, authToken, sourceConfig.Auth.Type)
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

// loadFromHTTP loads data from an HTTP URL, applying authentication if token is provided
func (rm *RegistryManager) loadFromHTTP(url string, authToken string, authType string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add cache-busting headers
	req.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Set("Pragma", "no-cache")

	// Add authentication header if token is provided
	if authToken != "" {
		switch strings.ToLower(authType) {
		case "basic":
			// authToken should be base64 encoded "username:password"
			req.Header.Set("Authorization", "Basic "+authToken)
		case "bearer", "": // Default to Bearer if type is not specified
			req.Header.Set("Authorization", "Bearer "+authToken)
		default:
			return nil, fmt.Errorf("unsupported authentication type for HTTP source: %s", authType)
		}
	}

	resp, err := rm.httpClient.Do(req)
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

	// Read custom sources from file if path is provided via environment variable
	customSourcesPath := os.Getenv("MCP_CUSTOM_REGISTRY_SOURCES_PATH")
	if customSourcesPath != "" {
		log.Printf("RegistryManager: Loading custom registry sources from file: %s", customSourcesPath)
		data, err := os.ReadFile(customSourcesPath)
		if err != nil {
			log.Printf("RegistryManager: Failed to read custom registry sources file %s: %v", customSourcesPath, err)
		} else {
			var fileSources []models.RegistrySourceConfig
			if err := json.Unmarshal(data, &fileSources); err != nil {
				log.Printf("RegistryManager: Failed to parse custom registry sources from %s: %v", customSourcesPath, err)
			} else {
				rm.customSources = append(rm.customSources, fileSources...)
				log.Printf("RegistryManager: Loaded %d custom registry sources from file %s", len(fileSources), customSourcesPath)
			}
		}
	}

	// Sync custom sources
	for _, source := range rm.customSources {
		if err := rm.LoadFromCustomSource(source); err != nil {
			log.Printf("RegistryManager: Failed to sync custom source %s: %v", source.URL, err)
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
