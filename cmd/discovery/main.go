package main

import (
	"fmt"
	"os"

	"suse-ai-up/pkg/services/discovery"
)

func main() {
	// Check if we're being called as a subcommand
	if len(os.Args) > 1 && os.Args[1] == "discovery" {
		// Shift arguments to remove the subcommand name
		os.Args = append(os.Args[:1], os.Args[2:]...)
		Main()
		return
	}
	Main()
}

func Main() {
	config := &discovery.Config{
		Port:           8912,
		DefaultTimeout: 30 * 1000000000, // 30 seconds in nanoseconds
		MaxConcurrency: 10,
		ExcludeProxy:   true,
	}

	service := discovery.NewService(config)
	if err := service.Start(); err != nil {
		fmt.Printf("Failed to start discovery service: %v\n", err)
		os.Exit(1)
	}
}
