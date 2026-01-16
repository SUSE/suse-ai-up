package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func main() {
	data, err := os.ReadFile("config/mcp_registry.yaml")
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}

	var servers []map[string]interface{}
	if err := yaml.Unmarshal(data, &servers); err != nil {
		fmt.Printf("Error parsing YAML: %v\n", err)
		return
	}

	fmt.Printf("Parsed %d servers\n", len(servers))
	for i, s := range servers {
		fmt.Printf("Server %d: name=%v, has_meta=%v\n", i, s["name"], s["meta"] != nil)
		if meta, ok := s["meta"].(map[string]interface{}); ok {
			fmt.Printf("  Meta: %+v\n", meta)
		}
	}
}
