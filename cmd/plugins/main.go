package main

import (
	"fmt"
	"os"
	"strconv"
	"suse-ai-up/pkg/services/plugins"
)

func main() {
	port := 8914     // Default port
	tlsPort := 38914 // Default TLS port

	// Read environment variables if set
	if envPort := os.Getenv("PLUGINS_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			port = p
		}
	}
	if envTLSPort := os.Getenv("PLUGINS_TLS_PORT"); envTLSPort != "" {
		if p, err := strconv.Atoi(envTLSPort); err == nil {
			tlsPort = p
		}
	}

	config := &plugins.Config{
		Port:           port,
		TLSPort:        tlsPort,         // HTTPS port
		HealthInterval: 30 * 1000000000, // 30 seconds in nanoseconds
		AutoTLS:        true,            // Enable auto-generated TLS certificates
	}

	service := plugins.NewService(config)
	if err := service.Start(); err != nil {
		fmt.Printf("Failed to start plugins service: %v\n", err)
		os.Exit(1)
	}
}
