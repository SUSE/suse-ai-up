package main

import (
	"fmt"
	"suse-ai-up/internal/config"
)

func main() {
	cfg := config.LoadConfig()
	fmt.Printf("MCP_REGISTRY_URL: '%s'\n", cfg.MCPRegistryURL)
	fmt.Printf("RegistryTimeout: '%s'\n", cfg.RegistryTimeout)
}
