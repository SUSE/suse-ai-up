package main

import (
	"fmt"
	"os"

	"suse-ai-up/pkg/services/registry"
)

func main() {
	// Check if we're being called as a subcommand
	if len(os.Args) > 1 && os.Args[1] == "registry" {
		// Shift arguments to remove the subcommand name
		os.Args = append(os.Args[:1], os.Args[2:]...)
		Main()
		return
	}
	Main()
}

func Main() {
	config := &registry.Config{
		Port:           8913,
		EnableOfficial: true,
		EnableDocker:   true,
	}

	service := registry.NewService(config)
	if err := service.Start(); err != nil {
		fmt.Printf("Failed to start registry service: %v\n", err)
		os.Exit(1)
	}
}
