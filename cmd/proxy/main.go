package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"suse-ai-up/pkg/services/proxy"
)

func main() {
	// Check if we're being called as a subcommand
	if len(os.Args) > 1 && os.Args[1] == "proxy" {
		// Shift arguments to remove the subcommand name
		os.Args = append(os.Args[:1], os.Args[2:]...)
		Main()
		return
	}
	Main()
}

func Main() {
	configFile := flag.String("config", "config/mcp_servers.json", "Path to MCP servers configuration file")
	port := flag.Int("port", 8080, "Port to run the server on")
	flag.Parse()

	// Create service configuration
	config := &proxy.Config{
		Port:       *port,
		ConfigFile: *configFile,
	}

	// Create and start the proxy service
	service := proxy.NewService(config)

	fmt.Printf("Starting MCP Proxy Service on port %d\n", *port)
	fmt.Printf("Configuration file: %s\n", *configFile)

	if err := service.Start(); err != nil {
		log.Fatalf("Failed to start proxy service: %v", err)
	}
}
