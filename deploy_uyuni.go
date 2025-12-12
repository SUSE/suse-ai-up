package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	// The docker command to convert to kubectl run
	dockerCommand := "docker run -it --rm -e UYUNI_SERVER=http://dummy.domain.com -e UYUNI_USER=admin -e UYUNI_PASS=admin -e UYUNI_MCP_TRANSPORT=http -e UYUNI_MCP_HOST=0.0.0.0 ghcr.io/uyuni-project/mcp-server-uyuni:latest"

	// Kubeconfig path
	kubeconfigPath := "/Users/alessandrofesta/.lima/rancher/copied-from-guest/kubeconfig.yaml"

	// Namespace
	namespace := "suse-ai-up-mcp"

	// Parse the docker command
	image, envVars, err := parseDockerCommand(dockerCommand)
	if err != nil {
		fmt.Printf("Failed to parse docker command: %v\n", err)
		os.Exit(1)
	}

	// Build the kubectl run command
	args := []string{"run", "mcp-sidecar-uyuni",
		fmt.Sprintf("--image=%s", image),
		"--port=8000",
		"--expose",
		fmt.Sprintf("--namespace=%s", namespace)}

	// Add environment variables
	for key, value := range envVars {
		args = append(args, fmt.Sprintf("--env=%s=%s", key, value))
	}

	// Log the kubectl command
	fmt.Printf("Executing kubectl command:\n")
	fmt.Printf("kubectl")
	for _, arg := range args {
		fmt.Printf(" %s", arg)
	}
	fmt.Printf("\n\n")

	// Execute the kubectl command
	cmd := exec.Command("kubectl", args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Failed to execute kubectl run: %v\n", err)
		fmt.Printf("Output: %s\n", string(output))
		os.Exit(1)
	}

	fmt.Printf("Success! Container deployed.\n")
	fmt.Printf("Output: %s\n", string(output))
}

// parseDockerCommand parses a docker run command and extracts image and env vars
func parseDockerCommand(command string) (string, map[string]string, error) {
	envVars := make(map[string]string)
	var image string

	fmt.Printf("Parsing docker command: %s\n", command)

	// Split the command into parts
	parts := strings.Fields(command)
	if len(parts) < 2 || parts[0] != "docker" || parts[1] != "run" {
		return "", nil, fmt.Errorf("invalid docker run command format")
	}

	// Parse arguments
	for i := 2; i < len(parts); i++ {
		arg := parts[i]

		// Look for -e flag followed by KEY=VALUE
		if arg == "-e" && i+1 < len(parts) {
			envPair := parts[i+1]
			if eqIndex := strings.Index(envPair, "="); eqIndex > 0 {
				key := envPair[:eqIndex]
				value := envPair[eqIndex+1:]
				envVars[key] = value
				fmt.Printf("Found env var: %s=%s\n", key, value)
			}
			i++ // Skip the next argument as we've consumed it
		} else if !strings.HasPrefix(arg, "-") && image == "" {
			// This should be the image name
			image = arg
			fmt.Printf("Found image: %s\n", image)
		}
	}

	if image == "" {
		return "", nil, fmt.Errorf("no image found in docker command")
	}

	return image, envVars, nil
}
