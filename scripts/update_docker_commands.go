package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"suse-ai-up/pkg/discovery"
	"suse-ai-up/pkg/models"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run update_docker_commands.go <registry-file>")
		fmt.Println("Example: go run update_docker_commands.go config/comprehensive_mcp_servers.json")
		os.Exit(1)
	}

	registryFile := os.Args[1]

	// Load the registry
	data, err := os.ReadFile(registryFile)
	if err != nil {
		log.Fatalf("Failed to read registry file: %v", err)
	}

	var registry []models.MCPServer
	if err := json.Unmarshal(data, &registry); err != nil {
		log.Fatalf("Failed to parse registry JSON: %v", err)
	}

	fmt.Printf("Loaded %d MCP servers from registry\n", len(registry))

	// Create the extractor
	extractor := discovery.NewDockerCommandExtractor()

	// Update registry with Docker commands
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Println("Starting Docker command extraction...")
	if err := extractor.UpdateRegistryWithDockerCommands(ctx, &registry); err != nil {
		log.Fatalf("Failed to update registry: %v", err)
	}

	// Save the updated registry
	updatedData, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal updated registry: %v", err)
	}

	if err := os.WriteFile(registryFile, updatedData, 0644); err != nil {
		log.Fatalf("Failed to write updated registry: %v", err)
	}

	fmt.Printf("✅ Successfully updated registry with Docker commands\n")
	fmt.Printf("📄 Updated file: %s\n", registryFile)
}
