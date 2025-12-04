package main

import (
	"fmt"
	"os"

	"suse-ai-up/pkg/services/plugins"
)

func main() {
	// Check if we're being called as a subcommand
	if len(os.Args) > 1 && os.Args[1] == "plugins" {
		// Shift arguments to remove the subcommand name
		os.Args = append(os.Args[:1], os.Args[2:]...)
		Main()
		return
	}
	Main()
}

func Main() {
	config := &plugins.Config{
		Port:           8914,
		HealthInterval: 30 * 1000000000, // 30 seconds in nanoseconds
	}

	service := plugins.NewService(config)
	if err := service.Start(); err != nil {
		fmt.Printf("Failed to start plugins service: %v\n", err)
		os.Exit(1)
	}
}
