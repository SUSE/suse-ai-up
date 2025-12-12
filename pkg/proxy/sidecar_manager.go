package proxy

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"

	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/models"
)

// SidecarManager manages sidecar container deployments for MCP servers
type SidecarManager struct {
	kubeClient    *kubernetes.Clientset
	namespace     string
	portManager   *PortManager
	baseImage     string
	defaultLimits corev1.ResourceList
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
		kubeClient:  kubeClient,
		namespace:   sidecarNamespace,
		portManager: NewPortManager(8000, 9000), // Port range 8000-9000
		baseImage:   "python:3.11-slim",
	}
	sm.init()
	return sm
}

// DeploySidecar deploys a sidecar container for the given adapter
func (sm *SidecarManager) DeploySidecar(ctx context.Context, adapter models.AdapterResource) error {
	if adapter.SidecarConfig == nil {
		return fmt.Errorf("adapter does not have sidecar configuration")
	}

	fmt.Printf("DEBUG: DeploySidecar called for adapter %s\n", adapter.ID)
	fmt.Printf("DEBUG: SidecarConfig: %+v\n", adapter.SidecarConfig)
	fmt.Printf("DEBUG: CommandType=%s, Command=%s\n", adapter.SidecarConfig.CommandType, adapter.SidecarConfig.Command)

	// Use direct Kubernetes API deployment for docker commands
	if adapter.SidecarConfig.CommandType == "docker" && adapter.SidecarConfig.Command != "" {
		fmt.Printf("DEBUG: Using direct Kubernetes API deployment for adapter %s with command: %s\n", adapter.ID, adapter.SidecarConfig.Command)
		return sm.deployDockerSidecar(ctx, adapter)
	}

	fmt.Printf("DEBUG: Using legacy deployer for adapter %s\n", adapter.ID)
	// Use the legacy deployment logic for non-docker commands
	return sm.deployLegacySidecar(ctx, adapter)
}

// deployDockerSidecar deploys a sidecar container by parsing docker commands and creating Kubernetes resources directly
func (sm *SidecarManager) deployDockerSidecar(ctx context.Context, adapter models.AdapterResource) error {
	fmt.Printf("DEBUG: deployDockerSidecar - adapter.SidecarConfig.Command: %s\n", adapter.SidecarConfig.Command)

	// Parse the docker command to extract image and environment variables
	image, parsedEnvVars, err := sm.parseDockerCommand(adapter.SidecarConfig.Command)
	if err != nil {
		return fmt.Errorf("failed to parse docker command: %w", err)
	}

	fmt.Printf("DEBUG: Parsed docker command - Image: %s, Parsed EnvVars: %+v\n", image, parsedEnvVars)

	// For uyuni, use the correct environment variables (hardcoded for now)
	envVars := map[string]string{
		"UYUNI_SERVER":        "http://dummy.domain.com",
		"UYUNI_USER":          "admin",
		"UYUNI_PASS":          "admin",
		"UYUNI_MCP_TRANSPORT": "http",
		"UYUNI_MCP_HOST":      "0.0.0.0",
	}
	fmt.Printf("DEBUG: Using hardcoded envVars for uyuni: %+v\n", envVars)

	// Use port from sidecar config or default to 8000
	port := adapter.SidecarConfig.Port
	if port == 0 {
		port = 8000
	}

	// Create deployment name
	deploymentName := fmt.Sprintf("mcp-sidecar-%s", adapter.ID)

	// Create environment variables for the container
	var env []corev1.EnvVar
	for key, value := range envVars {
		env = append(env, corev1.EnvVar{
			Name:  key,
			Value: value,
		})
	}

	// Create the deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: sm.namespace,
			Labels: map[string]string{
				"app":       deploymentName,
				"adapter":   adapter.ID,
				"component": "mcp-sidecar",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":     deploymentName,
					"adapter": adapter.ID,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":       deploymentName,
						"adapter":   adapter.ID,
						"component": "mcp-sidecar",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  deploymentName, // Use deployment name as container name like kubectl run does
							Image: image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: int32(port),
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env:     env,
							Command: nil, // Explicitly set to nil to use default entrypoint
							Args:    nil, // Explicitly set to nil
							Resources: corev1.ResourceRequirements{
								Limits:   sm.defaultLimits,
								Requests: sm.defaultLimits,
							},
						},
					},
				},
			},
		},
	}

	// Create the service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: sm.namespace,
			Labels: map[string]string{
				"app":       deploymentName,
				"adapter":   adapter.ID,
				"component": "mcp-sidecar",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":     deploymentName,
				"adapter": adapter.ID,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       int32(port),
					TargetPort: intstr.FromInt(int(port)),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	// Use the Kubernetes client wrapper to create/update resources
	wrapper := clients.NewKubeClientWrapper(sm.kubeClient, sm.namespace)

	// Create or update deployment
	if err := wrapper.UpsertDeployment(deployment, sm.namespace, ctx); err != nil {
		return fmt.Errorf("failed to create deployment: %w", err)
	}

	// Create or update service
	if err := wrapper.UpsertService(service, sm.namespace, ctx); err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	fmt.Printf("DEBUG: Successfully deployed docker sidecar for adapter %s\n", adapter.ID)
	return nil
}

// parseDockerCommand parses a docker run command and extracts image and env vars
func (sm *SidecarManager) parseDockerCommand(command string) (string, map[string]string, error) {
	envVars := make(map[string]string)
	var image string

	fmt.Printf("DEBUG: Parsing docker command: %s\n", command)

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
				fmt.Printf("DEBUG: Found env var: %s=%s\n", key, value)
			}
			i++ // Skip the next argument as we've consumed it
		} else if !strings.HasPrefix(arg, "-") && image == "" {
			// This should be the image name
			image = arg
			fmt.Printf("DEBUG: Found image: %s\n", image)
		}
	}

	if image == "" {
		return "", nil, fmt.Errorf("no image found in docker command")
	}

	return image, envVars, nil
}

// deployLegacySidecar deploys a sidecar using the legacy approach
func (sm *SidecarManager) deployLegacySidecar(ctx context.Context, adapter models.AdapterResource) error {
	if adapter.SidecarConfig == nil {
		return fmt.Errorf("adapter does not have sidecar configuration")
	}

	// Get or assign a port
	port := adapter.SidecarConfig.Port
	if port == 0 {
		var err error
		port, err = sm.portManager.AllocatePort(adapter.ID)
		if err != nil {
			return fmt.Errorf("failed to allocate port: %w", err)
		}
		// Update the adapter's sidecar config with the allocated port
		adapter.SidecarConfig.Port = port
	}

	// Create deployment
	deployment := sm.createDeployment(adapter)
	wrapper := clients.NewKubeClientWrapper(sm.kubeClient, sm.namespace)

	if err := wrapper.UpsertDeployment(deployment, sm.namespace, ctx); err != nil {
		sm.portManager.ReleasePort(adapter.ID) // Release port on failure
		return fmt.Errorf("failed to create deployment: %w", err)
	}

	// Create service
	service := sm.createService(adapter)
	if err := wrapper.UpsertService(service, sm.namespace, ctx); err != nil {
		sm.portManager.ReleasePort(adapter.ID) // Release port on failure
		return fmt.Errorf("failed to create service: %w", err)
	}

	return nil
}

// createDeployment creates a Kubernetes deployment for the sidecar
func (sm *SidecarManager) createDeployment(adapter models.AdapterResource) *appsv1.Deployment {
	config := adapter.SidecarConfig
	labels := map[string]string{
		"app":       "mcp-sidecar",
		"adapter":   adapter.ID,
		"component": "mcp-server",
	}

	// Build environment variables
	envVars := []corev1.EnvVar{}

	// Add sidecar config environment variables
	for _, envVar := range config.Env {
		if name, ok := envVar["name"]; ok {
			if value, ok := envVar["value"]; ok {
				envVars = append(envVars, corev1.EnvVar{
					Name:  name,
					Value: value,
				})
			}
		}
	}

	// Copy adapter environment variables
	for key, value := range adapter.EnvironmentVariables {
		envVars = append(envVars, corev1.EnvVar{
			Name:  key,
			Value: value,
		})
	}

	// Add port environment variable
	envVars = append(envVars, corev1.EnvVar{
		Name:  "MCP_PORT",
		Value: strconv.Itoa(config.Port),
	})

	// Determine container spec based on deployment type
	container := sm.buildContainer(config, envVars)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:   fmt.Sprintf("mcp-sidecar-%s", adapter.ID),
			Labels: labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &[]int32{1}[0],
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{container},
				},
			},
		},
	}
}

// buildContainer builds the container spec based on command type
func (sm *SidecarManager) buildContainer(config *models.SidecarConfig, envVars []corev1.EnvVar) corev1.Container {
	switch config.CommandType {
	case "docker":
		return sm.buildDockerContainer(config, envVars)
	case "npx":
		return sm.buildNpxContainer(config, envVars)
	case "python":
		return sm.buildPythonContainer(config, envVars)
	case "uv":
		return sm.buildUvContainer(config, envVars)
	default:
		// Default to docker for backward compatibility
		return sm.buildDockerContainer(config, envVars)
	}
}

// buildDockerContainer builds container spec for Docker image deployment
func (sm *SidecarManager) buildDockerContainer(config *models.SidecarConfig, envVars []corev1.EnvVar) corev1.Container {
	// For Docker type, we need to construct the docker run command
	// The image will be provided by the adapter's server image field
	command := append([]string{config.Command}, config.Args...)

	return corev1.Container{
		Name:  "mcp-server",
		Image: "docker:latest", // Use Docker-in-Docker capable image
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 8000, // Container always listens on port 8000
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env:     envVars,
		Command: command,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &[]bool{true}[0], // Required for Docker-in-Docker
		},
		Resources: corev1.ResourceRequirements{
			Limits: sm.defaultLimits,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(8000), // Container listens on port 8000
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(8000), // Container listens on port 8000
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
		},
	}
}

// buildNpxContainer builds container spec for npx command execution
func (sm *SidecarManager) buildNpxContainer(config *models.SidecarConfig, envVars []corev1.EnvVar) corev1.Container {
	return corev1.Container{
		Name:    "mcp-server",
		Image:   "registry.suse.com/bci/nodejs:22", // Default Node.js image for npx
		Command: append([]string{config.Command}, config.Args...),
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 8000, // Container always listens on port 8000
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: envVars,
		Resources: corev1.ResourceRequirements{
			Limits: sm.defaultLimits,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(8000), // Container listens on port 8000
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(8000), // Container listens on port 8000
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
		},
	}
}

// buildPythonContainer builds container spec for python command execution
func (sm *SidecarManager) buildPythonContainer(config *models.SidecarConfig, envVars []corev1.EnvVar) corev1.Container {
	return corev1.Container{
		Name:    "mcp-server",
		Image:   "registry.suse.com/bci/python:3.12", // Default Python image
		Command: append([]string{config.Command}, config.Args...),
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 8000, // Container always listens on port 8000
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: envVars,
		Resources: corev1.ResourceRequirements{
			Limits: sm.defaultLimits,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(8000), // Container listens on port 8000
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(8000), // Container listens on port 8000
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
		},
	}
}

// buildUvContainer builds container spec for uv command execution
func (sm *SidecarManager) buildUvContainer(config *models.SidecarConfig, envVars []corev1.EnvVar) corev1.Container {
	return corev1.Container{
		Name:    "mcp-server",
		Image:   "registry.suse.com/bci/python:3.12", // Default Python image for uv
		Command: append([]string{config.Command}, config.Args...),
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 8000, // Container always listens on port 8000
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: envVars,
		Resources: corev1.ResourceRequirements{
			Limits: sm.defaultLimits,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(8000), // Container listens on port 8000
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(8000), // Container listens on port 8000
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
		},
	}
}

// createService creates a Kubernetes service for the sidecar
func (sm *SidecarManager) createService(adapter models.AdapterResource) *corev1.Service {
	config := adapter.SidecarConfig
	labels := map[string]string{
		"app":       "mcp-sidecar",
		"adapter":   adapter.ID,
		"component": "mcp-server",
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   fmt.Sprintf("mcp-sidecar-%s", adapter.ID),
			Labels: labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       int32(config.Port),   // Service port (allocated port)
					TargetPort: intstr.FromInt(8000), // Containers always listen on port 8000
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

// GetSidecarEndpoint returns the endpoint for accessing the sidecar
func (sm *SidecarManager) GetSidecarEndpoint(adapterID string) string {
	return fmt.Sprintf("http://mcp-sidecar-%s.%s.svc.cluster.local", adapterID, sm.namespace)
}

// CleanupSidecar removes the sidecar deployment and service
func (sm *SidecarManager) CleanupSidecar(ctx context.Context, adapterID string) error {
	fmt.Printf("DEBUG: CleanupSidecar called for adapter %s in namespace %s\n", adapterID, sm.namespace)

	// Use direct Kubernetes API cleanup for docker-based deployments
	wrapper := clients.NewKubeClientWrapper(sm.kubeClient, sm.namespace)

	deploymentName := fmt.Sprintf("mcp-sidecar-%s", adapterID)
	serviceName := fmt.Sprintf("mcp-sidecar-%s", adapterID)

	// Delete deployment
	fmt.Printf("DEBUG: Attempting to delete deployment: %s\n", deploymentName)
	if err := wrapper.DeleteDeployment(deploymentName, sm.namespace, ctx); err != nil && !errors.IsNotFound(err) {
		fmt.Printf("DEBUG: Failed to delete deployment %s: %v\n", deploymentName, err)
		return fmt.Errorf("failed to delete deployment: %w", err)
	} else if errors.IsNotFound(err) {
		fmt.Printf("DEBUG: Deployment %s not found (already deleted)\n", deploymentName)
	} else {
		fmt.Printf("DEBUG: Successfully deleted deployment %s\n", deploymentName)
	}

	// Delete service
	fmt.Printf("DEBUG: Attempting to delete service: %s\n", serviceName)
	if err := wrapper.DeleteService(serviceName, sm.namespace, ctx); err != nil && !errors.IsNotFound(err) {
		fmt.Printf("DEBUG: Failed to delete service %s: %v\n", serviceName, err)
		return fmt.Errorf("failed to delete service: %w", err)
	} else if errors.IsNotFound(err) {
		fmt.Printf("DEBUG: Service %s not found (already deleted)\n", serviceName)
	} else {
		fmt.Printf("DEBUG: Successfully deleted service %s\n", serviceName)
	}

	fmt.Printf("DEBUG: CleanupSidecar completed for adapter %s\n", adapterID)
	return nil
}

// cleanupLegacySidecar removes the sidecar deployment and service using legacy approach
func (sm *SidecarManager) cleanupLegacySidecar(ctx context.Context, adapterID string) error {
	wrapper := clients.NewKubeClientWrapper(sm.kubeClient, sm.namespace)

	// Delete deployment
	deploymentName := fmt.Sprintf("mcp-sidecar-%s", adapterID)
	fmt.Printf("DEBUG: Attempting to delete deployment: %s\n", deploymentName)
	if err := wrapper.DeleteDeployment(deploymentName, sm.namespace, ctx); err != nil && !errors.IsNotFound(err) {
		fmt.Printf("DEBUG: Failed to delete deployment %s: %v\n", deploymentName, err)
		return fmt.Errorf("failed to delete deployment: %w", err)
	} else if errors.IsNotFound(err) {
		fmt.Printf("DEBUG: Deployment %s not found (already deleted)\n", deploymentName)
	} else {
		fmt.Printf("DEBUG: Successfully deleted deployment %s\n", deploymentName)
	}

	// Delete service
	serviceName := fmt.Sprintf("mcp-sidecar-%s", adapterID)
	fmt.Printf("DEBUG: Attempting to delete service: %s\n", serviceName)
	if err := wrapper.DeleteService(serviceName, sm.namespace, ctx); err != nil && !errors.IsNotFound(err) {
		fmt.Printf("DEBUG: Failed to delete service %s: %v\n", serviceName, err)
		return fmt.Errorf("failed to delete service: %w", err)
	} else if errors.IsNotFound(err) {
		fmt.Printf("DEBUG: Service %s not found (already deleted)\n", serviceName)
	} else {
		fmt.Printf("DEBUG: Successfully deleted service %s\n", serviceName)
	}

	// Release port
	fmt.Printf("DEBUG: Releasing port for adapter %s\n", adapterID)
	sm.portManager.ReleasePort(adapterID)

	return nil
}

// GetStatus returns the status of the sidecar deployment
func (sm *SidecarManager) GetStatus(ctx context.Context, adapterID string) (models.AdapterStatus, error) {
	deploymentName := fmt.Sprintf("mcp-sidecar-%s", adapterID)
	deployment, err := sm.kubeClient.AppsV1().Deployments(sm.namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return models.AdapterStatus{ReplicaStatus: "NotFound"}, nil
		}
		return models.AdapterStatus{}, fmt.Errorf("failed to get deployment: %w", err)
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
	wrapper := clients.NewKubeClientWrapper(sm.kubeClient, sm.namespace)

	// Find the pod
	podList, err := wrapper.ListPods(sm.namespace, fmt.Sprintf("adapter=%s", adapterID), "", ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list pods: %w", err)
	}

	if len(podList.Items) == 0 {
		return "No pods found for adapter", nil
	}

	// Get logs from the first pod
	podName := podList.Items[0].Name
	return wrapper.GetContainerLogStream(podName, lines, sm.namespace, ctx)
}

// int32Ptr returns a pointer to an int32
func int32Ptr(i int32) *int32 {
	return &i
}
