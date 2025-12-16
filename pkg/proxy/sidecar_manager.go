package proxy

import (
	"context"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// DeploySidecar deploys a sidecar container for the given adapter
func (sm *SidecarManager) DeploySidecar(ctx context.Context, adapter models.AdapterResource) error {
	if adapter.SidecarConfig == nil {
		return fmt.Errorf("adapter does not have sidecar configuration")
	}

	fmt.Printf("SIDECAR_MANAGER: DeploySidecar called for adapter %s\n", adapter.ID)
	fmt.Printf("SIDECAR_MANAGER: SidecarConfig: %+v\n", adapter.SidecarConfig)
	fmt.Printf("SIDECAR_MANAGER: CommandType=%s, Command=%s\n", adapter.SidecarConfig.CommandType, adapter.SidecarConfig.Command)
	fmt.Printf("SIDECAR_MANAGER: EnvironmentVariables: %+v\n", adapter.EnvironmentVariables)

	// Use DockerDeployer for docker commands
	if adapter.SidecarConfig.CommandType == "docker" && adapter.SidecarConfig.Command != "" {
		fmt.Printf("SIDECAR_MANAGER: Using DockerDeployer for adapter %s\n", adapter.ID)
		return sm.dockerDeployer.DeployFromDockerCommandWithEnv(adapter.SidecarConfig.Command, adapter.ID, adapter.EnvironmentVariables)
	}

	fmt.Printf("SIDECAR_MANAGER: No deployment method available for adapter %s\n", adapter.ID)
	return fmt.Errorf("unsupported sidecar configuration: commandType=%s", adapter.SidecarConfig.CommandType)
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
