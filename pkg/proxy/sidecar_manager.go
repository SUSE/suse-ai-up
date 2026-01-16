package proxy

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"

	"suse-ai-up/pkg/deployer"
	"suse-ai-up/pkg/models"
)

// SidecarManager manages sidecar container deployments for MCP servers
type SidecarManager struct {
	kubeClient     *kubernetes.Clientset
	namespace      string
	portManager    *PortManager
	baseImage      string
	defaultLimits  corev1.ResourceList
	dockerDeployer *deployer.DockerDeployer
}

// init initializes the default resource limits
func (sm *SidecarManager) init() {
	sm.defaultLimits = corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("512Mi"),
		corev1.ResourceCPU:    resource.MustParse("500m"),
	}
}

// NewSidecarManager creates a new sidecar manager
func NewSidecarManager(kubeClient *kubernetes.Clientset, namespace string) *SidecarManager {
	// Use dedicated namespace for MCP sidecars
	sidecarNamespace := "suse-ai-up-mcp"
	sm := &SidecarManager{
		kubeClient:     kubeClient,
		namespace:      sidecarNamespace,
		portManager:    NewPortManager(8000, 9000), // Port range 8000-9000
		baseImage:      "python:3.11-slim",
		dockerDeployer: deployer.NewDockerDeployer(sidecarNamespace),
	}
	sm.init()
	return sm
}

// NewSidecarManagerWithoutClient creates a sidecar manager that works without Kubernetes client
// This is useful when kubectl is available but the Go client cannot connect due to TLS issues
func NewSidecarManagerWithoutClient(namespace string) *SidecarManager {
	// Use dedicated namespace for MCP sidecars
	sidecarNamespace := "suse-ai-up-mcp"
	sm := &SidecarManager{
		kubeClient:     nil, // No Go client
		namespace:      sidecarNamespace,
		portManager:    NewPortManager(8000, 9000), // Port range 8000-9000
		baseImage:      "python:3.11-slim",
		dockerDeployer: deployer.NewDockerDeployer(sidecarNamespace),
	}
	sm.init()
	return sm
}

// DeploySidecar deploys a sidecar container for the given adapter
func (sm *SidecarManager) DeploySidecar(ctx context.Context, adapter models.AdapterResource) error {
	fmt.Printf("SIDECAR_MANAGER: SidecarConfig: %+v\n", adapter.SidecarConfig)
	fmt.Printf("SIDECAR_MANAGER: CommandType=%s, Command=%s\n", adapter.SidecarConfig.CommandType, adapter.SidecarConfig.Command)
	fmt.Printf("SIDECAR_MANAGER: EnvironmentVariables: %+v\n", adapter.EnvironmentVariables)

	// If we have a Kubernetes client, use it directly for deployment
	if sm.kubeClient != nil {
		fmt.Printf("SIDECAR_MANAGER: Using Kubernetes Go client for adapter %s\n", adapter.ID)
		return sm.deployWithKubeClient(ctx, adapter)
	}

	// If running in-cluster but no client available, this is an error
	if sm.isInCluster() {
		return fmt.Errorf("running in-cluster but no Kubernetes client available - check service account permissions")
	}

	// Handle different command types
	switch adapter.SidecarConfig.CommandType {
	case "docker":
		if adapter.SidecarConfig.Command != "" {
			fmt.Printf("SIDECAR_MANAGER: Using DockerDeployer (kubectl) for adapter %s\n", adapter.ID)
			err := sm.dockerDeployer.DeployFromDockerCommandWithEnv(adapter.SidecarConfig.Command, adapter.ID, adapter.EnvironmentVariables)
			if err != nil {
				return fmt.Errorf("kubectl deployment failed - ensure kubectl is configured and authenticated: %w", err)
			}
			return nil
		}
	case "python":
		if adapter.SidecarConfig.Command != "" {
			fmt.Printf("SIDECAR_MANAGER: Deploying python sidecar for adapter %s\n", adapter.ID)
			return sm.deployPythonSidecar(ctx, adapter)
		}
	case "npx":
		if adapter.SidecarConfig.Command != "" {
			fmt.Printf("SIDECAR_MANAGER: Deploying npx sidecar for adapter %s\n", adapter.ID)
			return sm.deployNpxSidecar(ctx, adapter)
		}
	case "go":
		if adapter.SidecarConfig.Command != "" {
			fmt.Printf("SIDECAR_MANAGER: Deploying go sidecar for adapter %s\n", adapter.ID)
			return sm.deployGoSidecar(ctx, adapter)
		}
	case "http":
		// For HTTP commandType, no sidecar deployment needed - routing is handled by adapter
		fmt.Printf("SIDECAR_MANAGER: HTTP remote MCP server for adapter %s, no sidecar needed\n", adapter.ID)
		return nil
	default:
		fmt.Printf("SIDECAR_MANAGER: No deployment method available for adapter %s\n", adapter.ID)
		return fmt.Errorf("unsupported sidecar configuration: commandType=%s", adapter.SidecarConfig.CommandType)
	}

	return fmt.Errorf("empty command for sidecar configuration: commandType=%s", adapter.SidecarConfig.CommandType)
}

// isInCluster checks if we're running inside a Kubernetes cluster
func (sm *SidecarManager) isInCluster() bool {
	// Check for in-cluster environment variables
	return os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_SERVICE_PORT") != ""
}

// GetSidecarEndpoint returns the endpoint for accessing the sidecar
func (sm *SidecarManager) GetSidecarEndpoint(adapterID string) string {
	return fmt.Sprintf("http://mcp-sidecar-%s.%s.svc.cluster.local", adapterID, sm.namespace)
}

// CleanupSidecar removes the sidecar deployment and service
func (sm *SidecarManager) CleanupSidecar(ctx context.Context, adapterID string) error {
	fmt.Printf("DEBUG: CleanupSidecar called for adapter %s in namespace %s\n", adapterID, sm.namespace)

	// If we have a Kubernetes client, use it directly for cleanup
	if sm.kubeClient != nil {
		fmt.Printf("SIDECAR_MANAGER: Using Kubernetes Go client for cleanup of adapter %s\n", adapterID)
		return sm.cleanupWithKubeClient(ctx, adapterID)
	}

	// Fallback to kubectl-based cleanup for local development
	fmt.Printf("SIDECAR_MANAGER: Using kubectl for cleanup of adapter %s\n", adapterID)
	return sm.dockerDeployer.Cleanup(adapterID)
}

// GetStatus returns the status of a sidecar deployment
func (sm *SidecarManager) GetStatus(ctx context.Context, adapterID string) (models.AdapterStatus, error) {
	// If no Kubernetes client available, return unknown status
	if sm.kubeClient == nil {
		return models.AdapterStatus{
			ReplicaStatus: "unknown (no k8s client)",
		}, nil
	}

	deploymentName := fmt.Sprintf("mcp-sidecar-%s", adapterID)

	// Check if deployment exists using Kubernetes API
	deployment, err := sm.kubeClient.AppsV1().Deployments(sm.namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return models.AdapterStatus{
				ReplicaStatus: "not_found",
			}, nil
		}
		return models.AdapterStatus{}, fmt.Errorf("failed to get deployment status: %w", err)
	}

	// Convert Deployment status to AdapterStatus
	status := models.AdapterStatus{
		Image: deployment.Spec.Template.Spec.Containers[0].Image,
	}

	if deployment.Status.ReadyReplicas > 0 {
		ready := int(deployment.Status.ReadyReplicas)
		status.ReadyReplicas = &ready
		status.ReplicaStatus = "Ready"
	} else {
		status.ReplicaStatus = "Pending"
	}

	return status, nil
}

// GetLogs retrieves logs from the sidecar container
func (sm *SidecarManager) GetLogs(ctx context.Context, adapterID string, lines int64) (string, error) {
	// If no Kubernetes client available, try kubectl directly
	if sm.kubeClient == nil {
		return sm.getLogsViaKubectl(adapterID, lines)
	}

	podName := fmt.Sprintf("mcp-sidecar-%s", adapterID)

	// Get logs using Kubernetes API
	logOptions := &corev1.PodLogOptions{
		Container: podName,
		TailLines: &lines,
	}

	req := sm.kubeClient.CoreV1().Pods(sm.namespace).GetLogs(podName, logOptions)
	logStream, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %w", err)
	}
	defer logStream.Close()

	logs, err := io.ReadAll(logStream)
	if err != nil {
		return "", fmt.Errorf("failed to read logs: %w", err)
	}

	return string(logs), nil
}

// deployWithKubeClient deploys a sidecar using the Kubernetes Go client directly
func (sm *SidecarManager) deployWithKubeClient(ctx context.Context, adapter models.AdapterResource) error {
	if sm.kubeClient == nil {
		return fmt.Errorf("kubernetes client not available")
	}

	// Handle different command types
	switch adapter.SidecarConfig.CommandType {
	case "docker":
		return sm.deployDockerWithKubeClient(ctx, adapter)
	case "python", "npx", "go":
		return sm.deployGenericWithKubeClient(ctx, adapter, sm.getImageForCommandType(adapter.SidecarConfig.CommandType), adapter.SidecarConfig.Command)
	default:
		return fmt.Errorf("unsupported command type: %s", adapter.SidecarConfig.CommandType)
	}
}

// deployDockerWithKubeClient deploys a docker-based sidecar using the Kubernetes Go client
func (sm *SidecarManager) deployDockerWithKubeClient(ctx context.Context, adapter models.AdapterResource) error {
	// Parse the docker command to extract image and environment variables
	image, envVars, port, err := sm.parseDockerCommand(adapter.SidecarConfig.Command)
	if err != nil {
		return fmt.Errorf("failed to parse docker command: %w", err)
	}

	// Merge additional environment variables
	if adapter.EnvironmentVariables != nil {
		for key, value := range adapter.EnvironmentVariables {
			envVars[key] = value
		}
	}

	fmt.Printf("SIDECAR_MANAGER: Deploying with Go client - image: %s, port: %d, envVars: %+v\n", image, port, envVars)

	// Create deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("mcp-sidecar-%s", adapter.ID),
			Namespace: sm.namespace,
			Labels: map[string]string{
				"app":       "mcp-sidecar",
				"adapterId": adapter.ID,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &[]int32{1}[0],
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":       "mcp-sidecar",
					"adapterId": adapter.ID,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":       "mcp-sidecar",
						"adapterId": adapter.ID,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "mcp-server",
							Image: image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: int32(port),
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: sm.buildEnvVarsWithOverrides(envVars),
							Resources: corev1.ResourceRequirements{
								Limits: sm.defaultLimits,
							},
						},
					},
				},
			},
		},
	}

	// Create the deployment
	_, err = sm.kubeClient.AppsV1().Deployments(sm.namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create deployment: %w", err)
	}

	// Create service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("mcp-sidecar-%s", adapter.ID),
			Namespace: sm.namespace,
			Labels: map[string]string{
				"app":       "mcp-sidecar",
				"adapterId": adapter.ID,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":       "mcp-sidecar",
				"adapterId": adapter.ID,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       int32(port),
					TargetPort: intstr.FromInt(port),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	_, err = sm.kubeClient.CoreV1().Services(sm.namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		// Try to clean up the deployment if service creation fails
		sm.kubeClient.AppsV1().Deployments(sm.namespace).Delete(ctx, deployment.Name, metav1.DeleteOptions{})
		return fmt.Errorf("failed to create service: %w", err)
	}

	fmt.Printf("SIDECAR_MANAGER: Successfully deployed docker sidecar for adapter %s\n", adapter.ID)
	return nil
}

// cleanupWithKubeClient removes the sidecar deployment and service using Kubernetes Go client
func (sm *SidecarManager) cleanupWithKubeClient(ctx context.Context, adapterID string) error {
	if sm.kubeClient == nil {
		return fmt.Errorf("kubernetes client not available")
	}

	deploymentName := fmt.Sprintf("mcp-sidecar-%s", adapterID)
	serviceName := fmt.Sprintf("mcp-sidecar-%s", adapterID)

	fmt.Printf("SIDECAR_MANAGER: Cleaning up deployment %s and service %s in namespace %s\n", deploymentName, serviceName, sm.namespace)

	// Delete the service first
	err := sm.kubeClient.CoreV1().Services(sm.namespace).Delete(ctx, serviceName, metav1.DeleteOptions{})
	if err != nil {
		// Log but don't fail if service doesn't exist
		fmt.Printf("SIDECAR_MANAGER: Warning: failed to delete service %s: %v\n", serviceName, err)
	}

	// Delete the deployment
	err = sm.kubeClient.AppsV1().Deployments(sm.namespace).Delete(ctx, deploymentName, metav1.DeleteOptions{})
	if err != nil {
		// Log but don't fail if deployment doesn't exist
		fmt.Printf("SIDECAR_MANAGER: Warning: failed to delete deployment %s: %v\n", deploymentName, err)
	}

	fmt.Printf("SIDECAR_MANAGER: Successfully initiated cleanup for adapter %s\n", adapterID)
	return nil
}

// parseDockerCommand parses a docker run command (same as DockerDeployer)
func (sm *SidecarManager) parseDockerCommand(command string) (string, map[string]string, int, error) {
	envVars := make(map[string]string)
	var image string
	port := 8000 // default port

	fmt.Printf("SIDECAR_MANAGER: Parsing docker command: %s\n", command)

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
				fmt.Printf("SIDECAR_MANAGER: Found env var: %s=%s\n", key, value)
			}
			i++ // Skip the next argument as we've consumed it
		} else if !strings.HasPrefix(arg, "-") && image == "" {
			// This should be the image name (last non-flag argument)
			image = arg
			fmt.Printf("SIDECAR_MANAGER: Found image: %s\n", image)
		}
	}

	if image == "" {
		return "", nil, 0, fmt.Errorf("no image found in docker command")
	}

	fmt.Printf("SIDECAR_MANAGER: Final env vars: %+v\n", envVars)
	return image, envVars, port, nil
}

// buildEnvVars converts map to Kubernetes env var format
func (sm *SidecarManager) buildEnvVars(envMap map[string]string) []corev1.EnvVar {
	return sm.buildEnvVarsWithOverrides(envMap)
}

// buildEnvVarsWithOverrides converts map to Kubernetes env var format with additional overrides
func (sm *SidecarManager) buildEnvVarsWithOverrides(envMap map[string]string) []corev1.EnvVar {
	var envVars []corev1.EnvVar
	for key, value := range envMap {
		envVars = append(envVars, corev1.EnvVar{
			Name:  key,
			Value: value,
		})
	}

	// Add UVICORN_HOST to ensure FastMCP servers bind to 0.0.0.0 instead of 127.0.0.1
	envVars = append(envVars, corev1.EnvVar{
		Name:  "UVICORN_HOST",
		Value: "0.0.0.0",
	})

	return envVars
}

// getLogsViaKubectl gets logs using kubectl command directly
func (sm *SidecarManager) getLogsViaKubectl(adapterID string, lines int64) (string, error) {
	// Use kubectl logs command
	args := []string{"logs", fmt.Sprintf("mcp-sidecar-%s", adapterID),
		"--namespace", sm.namespace,
		"--tail", fmt.Sprintf("%d", lines),
		"--ignore-errors"}

	cmd := exec.Command("kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get logs via kubectl: %w, output: %s", err, string(output))
	}

	return string(output), nil
}

// deployPythonSidecar deploys a python-based sidecar using the BCI python image
func (sm *SidecarManager) deployPythonSidecar(ctx context.Context, adapter models.AdapterResource) error {
	pythonImage := "registry.suse.com/bci/python:3.11"
	return sm.deployGenericSidecar(ctx, adapter, pythonImage, adapter.SidecarConfig.Command)
}

// deployNpxSidecar deploys an npx-based sidecar using the BCI nodejs image
func (sm *SidecarManager) deployNpxSidecar(ctx context.Context, adapter models.AdapterResource) error {
	nodejsImage := "registry.suse.com/bci/nodejs:22"
	return sm.deployGenericSidecar(ctx, adapter, nodejsImage, adapter.SidecarConfig.Command)
}

// deployGoSidecar deploys a go-based sidecar using the BCI golang image
func (sm *SidecarManager) deployGoSidecar(ctx context.Context, adapter models.AdapterResource) error {
	golangImage := "registry.suse.com/bci/golang:1.25"
	return sm.deployGenericSidecar(ctx, adapter, golangImage, adapter.SidecarConfig.Command)
}

// prepareGoReleaseCommand prepares a command to download and run a pre-built binary from GitHub/Gitea releases
func (sm *SidecarManager) prepareGoReleaseCommand(adapter models.AdapterResource, originalCommand string) string {
	projectURL := adapter.SidecarConfig.ProjectURL

	// Extract owner/repo from project URL
	var owner, repo, apiBase string
	if strings.Contains(projectURL, "github.com") {
		parts := strings.Split(strings.TrimSuffix(projectURL, ".git"), "/")
		if len(parts) >= 2 {
			owner = parts[len(parts)-2]
			repo = parts[len(parts)-1]
			apiBase = "https://api.github.com"
		}
	} else if strings.Contains(projectURL, "gitea.com") {
		parts := strings.Split(strings.TrimSuffix(projectURL, ".git"), "/")
		if len(parts) >= 2 {
			owner = parts[len(parts)-2]
			repo = parts[len(parts)-1]
			apiBase = "https://gitea.com/api/v1"
		}
	}

	if owner == "" || repo == "" {
		// Fallback to git clone + build
		return fmt.Sprintf("bash -c 'echo \"Failed to parse repo, falling back to build\" && zypper -n in git && git clone %s && cd %s && go build -o %s && ./%s'",
			projectURL, repo, originalCommand, originalCommand)
	}

	// Extract binary name from command
	cmdParts := strings.Fields(originalCommand)
	binaryName := cmdParts[0]

	// Create the download script - use simple string concatenation to avoid sprintf issues
	script := `bash -c '
echo "Downloading pre-built binary for ` + owner + `/` + repo + `..."
echo "API Base: ` + apiBase + `"

zypper -n in curl tar unzip

RELEASE_INFO=$(curl -s "` + apiBase + `/repos/` + owner + `/` + repo + `/releases/latest")

PLATFORM="Linux_arm64"
ARCHIVE_NAME="` + binaryName + `_${PLATFORM}.tar.gz"

echo "Looking for archive: $ARCHIVE_NAME"

DOWNLOAD_URL=$(echo "$RELEASE_INFO" | grep -o "https://[^\"]*${ARCHIVE_NAME}" | head -1)

if [ -n "$DOWNLOAD_URL" ]; then
    echo "Found binary archive: $DOWNLOAD_URL"
    curl -L -o binary.tar.gz "$DOWNLOAD_URL"
    if [ $? -eq 0 ]; then
        echo "Download successful, extracting..."
        tar -xzf binary.tar.gz
        if [ -f "` + binaryName + `" ]; then
            chmod +x ` + binaryName + `
            echo "Moving binary to /usr/bin..."
            mv ` + binaryName + ` /usr/bin/
            echo "Binary extracted successfully, starting server..."
            ` + originalCommand + `
        else
            echo "Binary ` + binaryName + ` not found, falling back to build..."
            zypper -n in git && git clone ` + projectURL + ` && cd ` + repo + ` && go build -o ` + binaryName + ` && ./` + originalCommand + `
        fi
    else
        echo "Download failed, falling back to build..."
        zypper -n in git && git clone ` + projectURL + ` && cd ` + repo + ` && go build -o ` + binaryName + ` && ./` + originalCommand + `
    fi
else
    echo "No suitable binary found, falling back to build..."
    zypper -n in git && git clone ` + projectURL + ` && cd ` + repo + ` && go build -o ` + binaryName + ` && ./` + originalCommand + `
fi
'`

	return script
}

// detectPlatform returns a platform string for binary matching
func (sm *SidecarManager) detectPlatform() string {
	// For now, assume linux amd64/arm64
	// In a real implementation, this would detect the actual platform
	return "linux-amd64"
}

// deployGenericSidecar deploys a sidecar using a generic container image and command
func (sm *SidecarManager) deployGenericSidecar(ctx context.Context, adapter models.AdapterResource, image, command string) error {
	// If we have a Kubernetes client, use it directly for deployment
	if sm.kubeClient != nil {
		fmt.Printf("SIDECAR_MANAGER: Using Kubernetes Go client for generic adapter %s\n", adapter.ID)
		return sm.deployGenericWithKubeClient(ctx, adapter, image, command)
	}

	// Fallback to kubectl-based deployment for local development
	fmt.Printf("SIDECAR_MANAGER: Using kubectl for generic adapter %s\n", adapter.ID)
	return sm.deployGenericWithKubectl(adapter, image, command)
}

// deployGenericWithKubeClient deploys a generic sidecar using the Kubernetes Go client directly
func (sm *SidecarManager) deployGenericWithKubeClient(ctx context.Context, adapter models.AdapterResource, image, command string) error {
	var err error
	if sm.kubeClient == nil {
		return fmt.Errorf("kubernetes client not available")
	}

	// Get port from sidecar config, default to 8000
	port := adapter.SidecarConfig.Port
	if port == 0 {
		port = 8000
	}

	// Merge additional environment variables
	envVars := make(map[string]string)
	if adapter.EnvironmentVariables != nil {
		for key, value := range adapter.EnvironmentVariables {
			envVars[key] = value
		}
	}

	fmt.Printf("SIDECAR_MANAGER: Deploying generic with Go client - image: %s, port: %d, command: %s, envVars: %+v\n", image, port, command, envVars)

	// Prepare the command with dependencies for python
	finalCommand := command
	if adapter.SidecarConfig.CommandType == "python" {
		// For python commands, we need to:
		// 1. Install uv
		// 2. Install git
		// 3. Clone the repository
		// 4. Run uv sync
		// 5. Run the original command
		projectURL := adapter.SidecarConfig.ProjectURL
		if projectURL != "" {
			// Extract repo name from URL
			parts := strings.Split(projectURL, "/")
			repoName := parts[len(parts)-1]
			if strings.HasSuffix(repoName, ".git") {
				repoName = repoName[:len(repoName)-4]
			}

			setupScript := fmt.Sprintf(
				"pip install uv && zypper -n in git && git clone %s && cd %s && uv sync && %s",
				projectURL,
				repoName,
				command,
			)
			finalCommand = setupScript
		} else {
			// Fallback: just install uv
			finalCommand = fmt.Sprintf("pip install uv && %s", command)
		}
	}

	// Prepare the command for Go projects
	if adapter.SidecarConfig.CommandType == "go" {
		// Check if we should use pre-built binaries from releases
		if adapter.SidecarConfig.ReleaseURL != "" {
			// Use pre-built binary from GitHub releases
			finalCommand = sm.prepareGoReleaseCommand(adapter, command)
		} else {
			// Build from source: git clone + go build
			projectURL := adapter.SidecarConfig.ProjectURL
			if projectURL != "" {
				// Extract repo name from URL
				parts := strings.Split(projectURL, "/")
				repoName := parts[len(parts)-1]
				if strings.HasSuffix(repoName, ".git") {
					repoName = repoName[:len(repoName)-4]
				}

				// Extract binary name from command (first word)
				cmdParts := strings.Fields(command)
				if len(cmdParts) == 0 {
					return fmt.Errorf("invalid Go command: %s", command)
				}
				binaryName := cmdParts[0]

				setupScript := fmt.Sprintf(
					"bash -c 'zypper -n in git && git clone %s && cd %s && echo \"Building...\" && go build -o %s && echo \"Build complete, starting server...\" && ./%s'",
					projectURL,
					repoName,
					binaryName,
					command,
				)
				finalCommand = setupScript
			} else {
				// Fallback: assume current directory has go.mod
				cmdParts := strings.Fields(command)
				if len(cmdParts) == 0 {
					return fmt.Errorf("invalid Go command: %s", command)
				}
				binaryName := cmdParts[0]
				finalCommand = fmt.Sprintf("go build -o %s && ./%s", binaryName, command)
			}
		}
	}

	// Create deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("mcp-sidecar-%s", adapter.ID),
			Namespace: sm.namespace,
			Labels: map[string]string{
				"app":       "mcp-sidecar",
				"adapterId": adapter.ID,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &[]int32{1}[0],
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":       "mcp-sidecar",
					"adapterId": adapter.ID,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":       "mcp-sidecar",
						"adapterId": adapter.ID,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "mcp-server",
							Image:   image,
							Command: []string{"sh", "-c", finalCommand}, // Execute command in shell
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: int32(port),
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: sm.buildEnvVarsWithOverrides(envVars),
							Resources: corev1.ResourceRequirements{
								Limits: sm.defaultLimits,
							},
						},
					},
				},
			},
		},
	}

	// Create the deployment
	_, err = sm.kubeClient.AppsV1().Deployments(sm.namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create deployment: %w", err)
	}

	// Create service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("mcp-sidecar-%s", adapter.ID),
			Namespace: sm.namespace,
			Labels: map[string]string{
				"app":       "mcp-sidecar",
				"adapterId": adapter.ID,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":       "mcp-sidecar",
				"adapterId": adapter.ID,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       int32(port),
					TargetPort: intstr.FromInt(port),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	_, err = sm.kubeClient.CoreV1().Services(sm.namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		// Try to clean up the deployment if service creation fails
		sm.kubeClient.AppsV1().Deployments(sm.namespace).Delete(ctx, deployment.Name, metav1.DeleteOptions{})
		return fmt.Errorf("failed to create service: %w", err)
	}

	fmt.Printf("SIDECAR_MANAGER: Successfully deployed generic sidecar for adapter %s\n", adapter.ID)
	return nil
}

// getImageForCommandType returns the appropriate container image for a command type
func (sm *SidecarManager) getImageForCommandType(commandType string) string {
	switch commandType {
	case "python":
		return "registry.suse.com/bci/python:3.11"
	case "npx":
		return "registry.suse.com/bci/nodejs:22"
	case "go":
		return "registry.suse.com/bci/golang:1.25"
	default:
		return "python:3.11-slim" // fallback
	}
}

// deployGenericWithKubectl deploys a generic sidecar using kubectl
func (sm *SidecarManager) deployGenericWithKubectl(adapter models.AdapterResource, image, command string) error {
	var err error
	// Get port from sidecar config, default to 8000
	port := adapter.SidecarConfig.Port
	if port == 0 {
		port = 8000
	}

	// Prepare the command with dependencies for python
	finalCommand := command
	if adapter.SidecarConfig.CommandType == "python" {
		// For python commands, we need to:
		// 1. Install uv
		// 2. Install git
		// 3. Clone the repository
		// 4. Run uv sync
		// 5. Run the original command
		projectURL := adapter.SidecarConfig.ProjectURL
		if projectURL != "" {
			// Extract repo name from URL
			parts := strings.Split(projectURL, "/")
			repoName := parts[len(parts)-1]
			if strings.HasSuffix(repoName, ".git") {
				repoName = repoName[:len(repoName)-4]
			}

			setupScript := fmt.Sprintf(
				"pip install uv && zypper -n in git && git clone %s && cd %s && uv sync && %s",
				projectURL,
				repoName,
				command,
			)
			finalCommand = setupScript
		} else {
			// Fallback: just install uv
			finalCommand = fmt.Sprintf("pip install uv && %s", command)
		}
	}

	// Build kubectl run command for generic container
	args := []string{"run", fmt.Sprintf("mcp-sidecar-%s", adapter.ID),
		fmt.Sprintf("--image=%s", image),
		fmt.Sprintf("--port=%d", port),
		"--expose",
		"--namespace", sm.namespace,
		"--command", "--", "sh", "-c", finalCommand} // Execute command in shell

	// Add environment variables
	if adapter.EnvironmentVariables != nil {
		for key, value := range adapter.EnvironmentVariables {
			args = append(args, fmt.Sprintf("--env=%s=%s", key, value))
		}
	}

	// Add insecure TLS skip for development
	kubeconfig := os.Getenv("KUBECONFIG")
	fmt.Printf("SIDECAR_MANAGER: kubeconfig='%s', checking for insecure skip\n", kubeconfig)
	if kubeconfig == "" || strings.Contains(kubeconfig, "localhost") || strings.Contains(kubeconfig, "127.0.0.1") {
		fmt.Printf("SIDECAR_MANAGER: Adding --insecure-skip-tls-verify for development\n")
		args = append([]string{"--insecure-skip-tls-verify"}, args...)
	}

	// Log the exact kubectl command being executed
	fmt.Printf("SIDECAR_MANAGER: kubectl command: kubectl")
	for _, arg := range args {
		fmt.Printf(" %s", arg)
	}
	fmt.Printf("\n")

	// Execute the kubectl command
	cmd := exec.Command("kubectl", args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfig))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to execute kubectl run: %w, output: %s", err, string(output))
	}

	fmt.Printf("SIDECAR_MANAGER: kubectl command completed successfully\n")
	return nil
}
