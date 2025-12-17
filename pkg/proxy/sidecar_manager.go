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
	if adapter.SidecarConfig == nil {
		return fmt.Errorf("adapter does not have sidecar configuration")
	}

	fmt.Printf("SIDECAR_MANAGER: DeploySidecar called for adapter %s\n", adapter.ID)
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

	// Fallback to kubectl-based deployment for local development
	if adapter.SidecarConfig.CommandType == "docker" && adapter.SidecarConfig.Command != "" {
		fmt.Printf("SIDECAR_MANAGER: Using DockerDeployer (kubectl) for adapter %s\n", adapter.ID)
		err := sm.dockerDeployer.DeployFromDockerCommandWithEnv(adapter.SidecarConfig.Command, adapter.ID, adapter.EnvironmentVariables)
		if err != nil {
			return fmt.Errorf("kubectl deployment failed - ensure kubectl is configured and authenticated: %w", err)
		}
		return nil
	}

	fmt.Printf("SIDECAR_MANAGER: No deployment method available for adapter %s\n", adapter.ID)
	return fmt.Errorf("unsupported sidecar configuration: commandType=%s", adapter.SidecarConfig.CommandType)
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

	// Use DockerDeployer cleanup (which uses kubectl delete)
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
							Env: sm.buildEnvVars(envVars),
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

	fmt.Printf("SIDECAR_MANAGER: Successfully deployed sidecar for adapter %s\n", adapter.ID)
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
	var envVars []corev1.EnvVar
	for key, value := range envMap {
		envVars = append(envVars, corev1.EnvVar{
			Name:  key,
			Value: value,
		})
	}
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
