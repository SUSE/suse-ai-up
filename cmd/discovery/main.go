package main

import (
	"fmt"
	"os"
	"strconv"
	"suse-ai-up/pkg/services/discovery"
)

func main() {
	port := 8912     // Default port
	tlsPort := 38912 // Default TLS port

	// Read environment variables if set
	if envPort := os.Getenv("DISCOVERY_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			port = p
		}
	}
	if envTLSPort := os.Getenv("DISCOVERY_TLS_PORT"); envTLSPort != "" {
		if p, err := strconv.Atoi(envTLSPort); err == nil {
			tlsPort = p
		}
	}

	config := &discovery.Config{
		Port:           port,
		TLSPort:        tlsPort,         // HTTPS port
		DefaultTimeout: 30 * 1000000000, // 30 seconds in nanoseconds
		MaxConcurrency: 10,
		ExcludeProxy:   true,
		AutoTLS:        true, // Enable auto-generated TLS certificates
	}

	service := discovery.NewService(config)
	if err := service.Start(); err != nil {
		fmt.Printf("Failed to start discovery service: %v\n", err)
		os.Exit(1)
	}
}
