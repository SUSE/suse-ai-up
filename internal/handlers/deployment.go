package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/models"
)

// DeploymentHandler handles MCP server deployment operations
type DeploymentHandler struct {
	Store      MCPServerStore
	KubeClient *clients.KubeClientWrapper
}

// NewDeploymentHandler creates a new deployment handler
func NewDeploymentHandler(store MCPServerStore, kubeClient *clients.KubeClientWrapper) *DeploymentHandler {
	return &DeploymentHandler{
		Store:      store,
		KubeClient: kubeClient,
	}
}

// GetMCPConfig handles GET /deployment/config/{serverId}
// @Summary Get MCP server configuration template
// @Description Retrieve the configuration template for deploying an MCP server
// @Tags deployment
// @Produce json
// @Param serverId path string true "MCP Server ID"
// @Success 200 {object} models.MCPConfigTemplate
// @Failure 404 {string} string "Server not found"
// @Router /api/v1/deployment/config/{serverId} [get]
func (h *DeploymentHandler) GetMCPConfig(c *gin.Context) {
	serverID := c.Param("serverId")
	// Remove leading slash if present
	serverID = strings.TrimPrefix(serverID, "/")

	// Get server from registry
	server, err := h.Store.GetMCPServer(serverID)
	if err != nil {
		log.Printf("MCP server not found: %s", serverID)
		c.JSON(http.StatusNotFound, gin.H{"error": "MCP server not found"})
		return
	}

	if server.ConfigTemplate == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No configuration template available for this server"})
		return
	}

	c.JSON(http.StatusOK, server.ConfigTemplate)
}

// DeployMCPDirect deploys an MCP server directly with the given parameters
func (h *DeploymentHandler) DeployMCPDirect(serverID string, envVars map[string]string, replicas int) error {
	req := DeployRequest{
		ServerID: serverID,
		EnvVars:  envVars,
		Replicas: replicas,
	}

	// Get server configuration
	server, err := h.Store.GetMCPServer(req.ServerID)
	if err != nil {
		log.Printf("MCP server not found: %s", req.ServerID)
		return fmt.Errorf("MCP server not found: %s", req.ServerID)
	}

	if server.ConfigTemplate == nil {
		return fmt.Errorf("Server does not have a deployment configuration")
	}

	// Validate that all required environment variables are provided
	if err := h.validateEnvironmentVariables(server.ConfigTemplate, req.EnvVars); err != nil {
		return err
	}

	// Deploy to Kubernetes
	_, err = h.deployToKubernetes(server, req)
	return err
}

// DeployMCP handles POST /deployment/deploy
// @Summary Deploy an MCP server
// @Description Deploy an MCP server to Kubernetes with provided configuration
// @Tags deployment
// @Accept json
// @Produce json
// @Param deployment body DeployRequest true "Deployment configuration"
// @Success 200 {object} DeployResponse
// @Failure 400 {string} string "Bad Request"
// @Router /api/v1/deployment/deploy [post]
func (h *DeploymentHandler) DeployMCP(c *gin.Context) {
	var req DeployRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("Error decoding deployment request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	// Validate request
	if req.ServerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "server_id is required"})
		return
	}

	// Get server configuration
	server, err := h.Store.GetMCPServer(req.ServerID)
	if err != nil {
		log.Printf("MCP server not found: %s", req.ServerID)
		c.JSON(http.StatusNotFound, gin.H{"error": "MCP server not found"})
		return
	}

	if server.ConfigTemplate == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Server does not have a deployment configuration"})
		return
	}

	// Validate that all required environment variables are provided
	if err := h.validateEnvironmentVariables(server.ConfigTemplate, req.EnvVars); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if Kubernetes client is available
	if h.KubeClient == nil {
		// Fallback to returning manifests if no Kubernetes client
		manifests, err := h.generateKubernetesManifests(server, req)
		if err != nil {
			log.Printf("Failed to generate Kubernetes manifests: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate deployment manifests"})
			return
		}

		response := DeployResponse{
			ServerID:     req.ServerID,
			DeploymentID: fmt.Sprintf("mcp-%s-%d", strings.ReplaceAll(req.ServerID, "/", "-"), 123456),
			Manifests:    manifests,
			Status:       "generated",
		}

		c.JSON(http.StatusOK, response)
		return
	}

	// Deploy to Kubernetes
	deploymentID, err := h.deployToKubernetes(server, req)
	if err != nil {
		log.Printf("Failed to deploy to Kubernetes: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to deploy to Kubernetes: %v", err)})
		return
	}

	response := DeployResponse{
		ServerID:     req.ServerID,
		DeploymentID: deploymentID,
		Manifests:    nil, // Not returning manifests when actually deployed
		Status:       "deployed",
	}

	c.JSON(http.StatusOK, response)
}

// DeployRequest represents a deployment request
type DeployRequest struct {
	ServerID  string            `json:"server_id" binding:"required"`
	EnvVars   map[string]string `json:"env_vars,omitempty"`
	Replicas  int               `json:"replicas,omitempty"`
	Resources *ResourceLimits   `json:"resources,omitempty"`
}

// ResourceLimits represents Kubernetes resource limits
type ResourceLimits struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

// DeployResponse represents a deployment response
type DeployResponse struct {
	ServerID     string            `json:"server_id"`
	DeploymentID string            `json:"deployment_id"`
	Manifests    map[string]string `json:"manifests"`
	Status       string            `json:"status"`
}

// generateKubernetesObjects generates Kubernetes objects for MCP server deployment
func (h *DeploymentHandler) generateKubernetesObjects(server *models.MCPServer, req DeployRequest) (*appsv1.Deployment, *corev1.Service, error) {
	if server.ConfigTemplate == nil {
		return nil, nil, fmt.Errorf("no configuration template available")
	}

	config := server.ConfigTemplate

	// Set defaults
	replicas := int32(req.Replicas)
	if replicas <= 0 {
		replicas = 1
	}

	// Generate deployment
	deploymentName := fmt.Sprintf("mcp-%s", strings.ReplaceAll(server.ID, "/", "-"))
	containerName := strings.ReplaceAll(server.ID, "/", "-")

	// Determine ports based on transport type and environment variables
	var containerPort int32
	var portName string
	switch config.Transport {
	case "http":
		// Check if PORT is specified in environment variables
		if portStr, exists := config.Env["PORT"]; exists {
			if port, err := strconv.Atoi(portStr); err == nil {
				containerPort = int32(port)
			} else {
				containerPort = 3000
			}
		} else {
			containerPort = 3000
		}
		portName = "http"
	case "sse":
		containerPort = 3000
		portName = "sse"
	default: // stdio
		containerPort = 3000
		portName = "stdio"
	}

	// Build environment variables
	var envVars []corev1.EnvVar
	for key, value := range config.Env {
		if userValue, exists := req.EnvVars[key]; exists {
			envVars = append(envVars, corev1.EnvVar{Name: key, Value: userValue})
		} else if value != "" {
			envVars = append(envVars, corev1.EnvVar{Name: key, Value: value})
		}
	}

	// Build args
	var args []string
	if len(config.Args) > 0 {
		args = config.Args
	}

	// Create deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: deploymentName,
			Labels: map[string]string{
				"app":           "mcp-server",
				"mcp-server-id": server.ID,
				"mcp-transport": config.Transport,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":           "mcp-server",
					"mcp-server-id": server.ID,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":           "mcp-server",
						"mcp-server-id": server.ID,
						"mcp-transport": config.Transport,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    containerName,
							Image:   config.Image,
							Command: []string{config.Command},
							Args:    args,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: containerPort,
									Name:          portName,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: envVars,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("128Mi"),
									corev1.ResourceCPU:    resource.MustParse("100m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("512Mi"),
									corev1.ResourceCPU:    resource.MustParse("500m"),
								},
							},
						},
					},
				},
			},
		},
	}

	// For virtualMCP servers, the template is assumed to be in the container image
	// TODO: Add logic to ensure template file is available in the container

	// Create service (only for HTTP/SSE transports)
	var service *corev1.Service
	if config.Transport == "http" || config.Transport == "sse" {
		service = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: deploymentName,
				Labels: map[string]string{
					"app":           "mcp-server",
					"mcp-server-id": server.ID,
					"mcp-transport": config.Transport,
				},
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{
					"app":           "mcp-server",
					"mcp-server-id": server.ID,
				},
				Ports: []corev1.ServicePort{
					{
						Name:       portName,
						Port:       80,
						TargetPort: intstr.FromString(portName),
						Protocol:   corev1.ProtocolTCP,
					},
				},
				Type: corev1.ServiceTypeClusterIP,
			},
		}
	}

	return deployment, service, nil
}

// generateKubernetesManifests generates Kubernetes manifests for MCP server deployment (fallback)
func (h *DeploymentHandler) generateKubernetesManifests(server *models.MCPServer, req DeployRequest) (map[string]string, error) {
	deployment, service, err := h.generateKubernetesObjects(server, req)
	if err != nil {
		return nil, err
	}

	manifests := make(map[string]string)

	// Convert deployment to YAML (simplified)
	deploymentYAML := fmt.Sprintf("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: %s\n...", deployment.Name)
	manifests["deployment.yaml"] = deploymentYAML

	if service != nil {
		serviceYAML := fmt.Sprintf("apiVersion: v1\nkind: Service\nmetadata:\n  name: %s\n...", service.Name)
		manifests["service.yaml"] = serviceYAML
	}

	return manifests, nil
}

// deployToKubernetes deploys the MCP server to Kubernetes
func (h *DeploymentHandler) deployToKubernetes(server *models.MCPServer, req DeployRequest) (string, error) {
	ctx := context.Background()
	deploymentID := fmt.Sprintf("mcp-%s-%d", strings.ReplaceAll(req.ServerID, "/", "-"), 123456) // TODO: generate proper ID

	// Generate Kubernetes objects
	deployment, service, err := h.generateKubernetesObjects(server, req)
	if err != nil {
		return "", fmt.Errorf("failed to generate Kubernetes objects: %v", err)
	}

	namespace := h.KubeClient.GetNamespace()

	// Deploy the Deployment
	if err := h.KubeClient.UpsertDeployment(deployment, namespace, ctx); err != nil {
		return "", fmt.Errorf("failed to create deployment: %v", err)
	}

	// Deploy the Service (if any)
	if service != nil {
		if err := h.KubeClient.UpsertService(service, namespace, ctx); err != nil {
			return "", fmt.Errorf("failed to create service: %v", err)
		}
	}

	log.Printf("Successfully deployed MCP server %s with deployment ID %s", req.ServerID, deploymentID)
	return deploymentID, nil
}

// validateEnvironmentVariables checks that all required environment variables are provided
func (h *DeploymentHandler) validateEnvironmentVariables(config *models.MCPConfigTemplate, providedEnv map[string]string) error {
	if config == nil || config.Env == nil {
		return nil // No env vars required
	}

	var missingVars []string
	for envKey := range config.Env {
		if _, provided := providedEnv[envKey]; !provided {
			missingVars = append(missingVars, envKey)
		}
	}

	if len(missingVars) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missingVars, ", "))
	}

	return nil
}
