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
	// Build the command array
	var command []string

	if config.DockerCommand != "" {
		// Use the full command as specified
		command = strings.Fields(config.DockerCommand)
	}

	return corev1.Container{
		Name:  "mcp-server",
		Image: config.DockerImage,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: int32(config.Port),
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env:     envVars,
		Command: command,
		Resources: corev1.ResourceRequirements{
			Limits: sm.defaultLimits,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(config.Port),
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(config.Port),
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
		Image:   config.BaseImage,
		Command: append([]string{"npx", config.Command}, config.Args...),
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: int32(config.Port),
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
					Port: intstr.FromInt(config.Port),
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(config.Port),
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
		Image:   config.BaseImage,
		Command: append([]string{"python", config.Command}, config.Args...),
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: int32(config.Port),
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
					Port: intstr.FromInt(config.Port),
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(config.Port),
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
		Image:   config.BaseImage,
		Command: append([]string{"uv", config.Command}, config.Args...),
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: int32(config.Port),
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
					Port: intstr.FromInt(config.Port),
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(config.Port),
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
					Port:       int32(config.Port),
					TargetPort: intstr.FromInt(config.Port),
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
	wrapper := clients.NewKubeClientWrapper(sm.kubeClient, sm.namespace)

	// Delete deployment
	deploymentName := fmt.Sprintf("mcp-sidecar-%s", adapterID)
	if err := wrapper.DeleteDeployment(deploymentName, sm.namespace, ctx); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}

	// Delete service
	serviceName := fmt.Sprintf("mcp-sidecar-%s", adapterID)
	if err := wrapper.DeleteService(serviceName, sm.namespace, ctx); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete service: %w", err)
	}

	// Release port
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
