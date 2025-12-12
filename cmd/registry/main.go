package main

import (
	"fmt"
	"os"
	"strconv"
	"suse-ai-up/pkg/services/registry"
)

func main() {
	port := 8913     // Default port
	tlsPort := 38913 // Default TLS port

	// Read environment variables if set
	if envPort := os.Getenv("REGISTRY_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			port = p
		}
	}
	if envTLSPort := os.Getenv("REGISTRY_TLS_PORT"); envTLSPort != "" {
		if p, err := strconv.Atoi(envTLSPort); err == nil {
			tlsPort = p
		}
	}

	config := &registry.Config{
		Port:              port,
		TLSPort:           tlsPort, // HTTPS port
		RemoteServersFile: "config/comprehensive_mcp_servers.yaml",
		AutoTLS:           true, // Enable auto-generated TLS certificates
	}

	service := registry.NewService(config)
	if err := service.Start(); err != nil {
		fmt.Printf("Failed to start registry service: %v\n", err)
		os.Exit(1)
	}
}
