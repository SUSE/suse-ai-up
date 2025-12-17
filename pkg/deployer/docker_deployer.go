package deployer

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// DockerDeployer handles deployment of docker-based MCP servers to Kubernetes using kubectl
type DockerDeployer struct {
	namespace string
}

// NewDockerDeployer creates a new DockerDeployer instance
func NewDockerDeployer(namespace string) *DockerDeployer {
	return &DockerDeployer{
		namespace: namespace,
	}
}

// DeployFromDockerCommand takes a docker run command and converts it to kubectl run
func (d *DockerDeployer) DeployFromDockerCommand(dockerCommand string, name string) error {
	fmt.Printf("DOCKER_DEPLOYER: DeployFromDockerCommand called with command: %s, name: %s\n", dockerCommand, name)
	return d.deployFromDockerCommand(dockerCommand, name, nil, false)
}

// DeployFromDockerCommandWithEnv takes a docker run command and additional env vars, then converts to kubectl run
func (d *DockerDeployer) DeployFromDockerCommandWithEnv(dockerCommand string, name string, additionalEnv map[string]string) error {
	return d.deployFromDockerCommand(dockerCommand, name, additionalEnv, false)
}

// DryRunFromDockerCommand shows the kubectl command without executing it
func (d *DockerDeployer) DryRunFromDockerCommand(dockerCommand string, name string) {
	d.deployFromDockerCommand(dockerCommand, name, nil, true)
}

func (d *DockerDeployer) deployFromDockerCommand(dockerCommand string, name string, additionalEnv map[string]string, dryRun bool) error {
	// Parse the docker command to extract image and environment variables
	image, envVars, port, err := d.parseDockerCommand(dockerCommand)
	if err != nil {
		return fmt.Errorf("failed to parse docker command: %w", err)
	}

	// Merge additional environment variables (additionalEnv takes precedence)
	if additionalEnv != nil {
		for key, value := range additionalEnv {
			envVars[key] = value
		}
	}

	// Build the kubectl run command
	args := []string{"run", fmt.Sprintf("mcp-sidecar-%s", name),
		fmt.Sprintf("--image=%s", image),
		fmt.Sprintf("--port=%d", port),
		"--expose",
		"--namespace", d.namespace}

	// Add environment variables individually
	for key, value := range envVars {
		args = append(args, fmt.Sprintf("--env=%s=%s", key, value))
	}

	// Add insecure TLS skip for development (when KUBECONFIG is not set or contains localhost)
	kubeconfig := os.Getenv("KUBECONFIG")
	fmt.Printf("DOCKER_DEPLOYER: kubeconfig='%s', checking for insecure skip\n", kubeconfig)
	if kubeconfig == "" || strings.Contains(kubeconfig, "localhost") || strings.Contains(kubeconfig, "127.0.0.1") {
		fmt.Printf("DOCKER_DEPLOYER: Adding --insecure-skip-tls-verify for development\n")
		args = append([]string{"--insecure-skip-tls-verify"}, args...)
	}

	// Log the exact kubectl command being executed
	fmt.Printf("DOCKER_DEPLOYER: kubectl command: kubectl")
	for _, arg := range args {
		fmt.Printf(" %s", arg)
	}
	fmt.Printf("\n")

	if dryRun {
		return nil
	}

	// Execute the kubectl command
	cmd := exec.Command("kubectl", args...)

	cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfig))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to execute kubectl run: %w, output: %s", err, string(output))
	}

	fmt.Printf("DOCKER_DEPLOYER: kubectl command completed successfully\n")
	return nil
}

// parseDockerCommand parses a docker run command and extracts image, env vars, and port
func (d *DockerDeployer) parseDockerCommand(command string) (string, map[string]string, int, error) {
	envVars := make(map[string]string)
	var image string
	port := 8000 // default port

	fmt.Printf("DEBUG: Parsing docker command: %s\n", command)

	// Split the command into parts
	parts := strings.Fields(command)
	if len(parts) < 2 || parts[0] != "docker" || parts[1] != "run" {
		return "", nil, 0, fmt.Errorf("invalid docker run command format")
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
				fmt.Printf("DEBUG: Found env var: %s=%s\n", key, value)
			}
			i++ // Skip the next argument as we've consumed it
		} else if !strings.HasPrefix(arg, "-") && image == "" {
			// This should be the image name (last non-flag argument)
			image = arg
			fmt.Printf("DEBUG: Found image: %s\n", image)
		}
	}

	if image == "" {
		return "", nil, 0, fmt.Errorf("no image found in docker command")
	}

	fmt.Printf("DEBUG: Final env vars: %+v\n", envVars)
	return image, envVars, port, nil
}

// Cleanup removes the deployment and service for the MCP server
func (d *DockerDeployer) Cleanup(name string) error {
	// Use kubectl to delete the deployment and service
	deploymentName := fmt.Sprintf("mcp-sidecar-%s", name)
	serviceName := fmt.Sprintf("mcp-sidecar-%s", name)

	kubeconfig := os.Getenv("KUBECONFIG")

	// Delete deployment
	args := []string{"delete", "deployment", deploymentName, "--namespace", d.namespace, "--ignore-not-found=true"}
	if kubeconfig == "" || strings.Contains(kubeconfig, "localhost") || strings.Contains(kubeconfig, "127.0.0.1") {
		args = append([]string{"--insecure-skip-tls-verify"}, args...)
	}
	cmd := exec.Command("kubectl", args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfig))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete deployment: %w, output: %s", err, string(output))
	}

	// Delete service
	args = []string{"delete", "service", serviceName, "--namespace", d.namespace, "--ignore-not-found=true"}
	if kubeconfig == "" || strings.Contains(kubeconfig, "localhost") || strings.Contains(kubeconfig, "127.0.0.1") {
		args = append([]string{"--insecure-skip-tls-verify"}, args...)
	}
	cmd = exec.Command("kubectl", args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfig))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete service: %w, output: %s", err, string(output))
	}

	return nil
}
