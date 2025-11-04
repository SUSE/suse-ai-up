package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type RegistryResponse struct {
	Servers  []Server `json:"servers"`
	Metadata struct {
		NextCursor string `json:"nextCursor"`
		Count      int    `json:"count"`
	} `json:"metadata"`
}

type Server struct {
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

type Package struct {
	RegistryType         string                `json:"registryType"`
	Identifier           string                `json:"identifier"`
	Transport            Transport             `json:"transport"`
	EnvironmentVariables []EnvironmentVariable `json:"environmentVariables,omitempty"`
}

type Remote struct {
	Type    string   `json:"type"`
	URL     string   `json:"url"`
	Headers []Header `json:"headers,omitempty"`
}

type Header struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IsSecret    bool   `json:"isSecret,omitempty"`
	Value       string `json:"value,omitempty"`
}

type Transport struct {
	Type string `json:"type"`
}

type EnvironmentVariable struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Format      string `json:"format,omitempty"`
	IsSecret    bool   `json:"isSecret,omitempty"`
	Default     string `json:"default,omitempty"`
}

func main() {
	targetProviders := []string{"microsoft", "atlassian", "github"}
	var foundServers []Server

	cursor := ""
	pageCount := 0

	fmt.Println("Searching MCP registry for servers from providers:", strings.Join(targetProviders, ", "))

	for {
		url := "https://registry.modelcontextprotocol.io/v0.1/servers?limit=100"
		if cursor != "" {
			url += "&cursor=" + cursor
		}

		fmt.Printf("\nFetching page %d (cursor: %s)...\n", pageCount+1, cursor)

		resp, err := http.Get(url)
		if err != nil {
			log.Fatalf("Error fetching registry: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			log.Fatalf("Registry returned status %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatalf("Error reading response: %v", err)
		}

		var registryResp RegistryResponse
		if err := json.Unmarshal(body, &registryResp); err != nil {
			log.Fatalf("Error parsing JSON: %v", err)
		}

		// Filter servers for target providers
		for _, server := range registryResp.Servers {
			serverName := strings.ToLower(server.Server.Name)
			for _, provider := range targetProviders {
				if strings.Contains(serverName, provider) {
					fmt.Printf("Found server: %s\n", server.Server.Name)
					foundServers = append(foundServers, server)
					break
				}
			}
		}

		pageCount++
		fmt.Printf("Processed %d servers on this page, total found: %d\n", len(registryResp.Servers), len(foundServers))

		// Check if there are more pages
		if registryResp.Metadata.NextCursor == "" || registryResp.Metadata.NextCursor == cursor {
			break
		}

		cursor = registryResp.Metadata.NextCursor

		// Small delay to be respectful to the API
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Printf("\n=== SEARCH COMPLETE ===\n")
	fmt.Printf("Total pages processed: %d\n", pageCount)
	fmt.Printf("Total servers found: %d\n\n", len(foundServers))

	if len(foundServers) == 0 {
		fmt.Println("No servers found from the specified providers.")
		return
	}

	// Output found servers in JSON format for further processing
	fmt.Println("Found servers:")
	for i, server := range foundServers {
		fmt.Printf("\n%d. %s\n", i+1, server.Server.Name)
		fmt.Printf("   Description: %s\n", server.Server.Description)
		fmt.Printf("   Version: %s\n", server.Server.Version)
		if server.Server.Repository != nil {
			fmt.Printf("   Repository: %s (%s)\n", server.Server.Repository.URL, server.Server.Repository.Source)
		}
	}

	// Save to JSON file for publishing
	output := map[string]interface{}{
		"providers": targetProviders,
		"servers":   foundServers,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	outputJSON, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		log.Fatalf("Error marshaling output: %v", err)
	}

	err = writeFile("found_servers.json", outputJSON)
	if err != nil {
		log.Fatalf("Error writing output file: %v", err)
	}

	fmt.Printf("\nResults saved to found_servers.json\n")
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
