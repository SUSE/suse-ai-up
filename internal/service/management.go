package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/session"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/gin-gonic/gin"
)

// AdapterResponse represents a successful adapter creation/update response
type AdapterResponse struct {
	models.AdapterResource
}

// AdapterListResponse represents a list of adapters
type AdapterListResponse []models.AdapterResource

// AdapterStatusResponse represents adapter status information
type AdapterStatusResponse struct {
	models.AdapterStatus
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// ManagementService handles adapter management
type ManagementService struct {
	kubeClient   *clients.KubeClientWrapper
	store        clients.AdapterResourceStore
	sessionStore session.SessionStore
	mcpDiscovery *MCPDiscoveryService
}

// NewManagementService creates a new ManagementService
func NewManagementService(kubeClient *clients.KubeClientWrapper, store clients.AdapterResourceStore, sessionStore session.SessionStore) *ManagementService {
	return &ManagementService{
		kubeClient:   kubeClient,
		store:        store,
		sessionStore: sessionStore,
		mcpDiscovery: NewMCPDiscoveryService(),
	}
}

// CreateAdapter handles POST /adapters
// @Summary Create a new MCP server adapter
// @Description Creates a new MCP adapter and deploys it to Kubernetes.
// @Tags adapters
// @Accept json
// @Produce json
// @Param body body models.AdapterData true "Adapter configuration"
// @Success 201 {object} models.AdapterResource
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/adapters [post]
func (ms *ManagementService) CreateAdapter(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in CreateAdapter: %v", r)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		}
	}()
	log.Printf("CreateAdapter: Starting adapter creation")
	var req struct {
		Name                 string                    `json:"name"`
		ImageName            string                    `json:"imageName"`
		ImageVersion         string                    `json:"imageVersion"`
		Protocol             string                    `json:"protocol"`
		ConnectionType       string                    `json:"connectionType"`
		EnvironmentVariables map[string]string         `json:"environmentVariables"`
		ReplicaCount         int                       `json:"replicaCount"`
		Description          string                    `json:"description"`
		UseWorkloadIdentity  bool                      `json:"useWorkloadIdentity"`
		RemoteUrl            string                    `json:"remoteUrl"`
		Command              string                    `json:"command"`
		Args                 []string                  `json:"args"`
		MCPClientConfig      models.MCPClientConfig    `json:"mcpClientConfig"`
		Authentication       *models.AdapterAuthConfig `json:"authentication"`
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("CreateAdapter: Failed to read body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}
	if err := json.Unmarshal(body, &req); err != nil {
		log.Printf("CreateAdapter: JSON unmarshal error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	log.Printf("CreateAdapter: JSON bound successfully")
	log.Printf("DEBUG: req.ConnectionType: %s", req.ConnectionType)

	var data models.AdapterData
	data.Name = req.Name
	data.ImageName = req.ImageName
	data.ImageVersion = req.ImageVersion
	data.Protocol = models.ServerProtocol(req.Protocol)
	data.ConnectionType = models.ConnectionType(req.ConnectionType)
	data.EnvironmentVariables = req.EnvironmentVariables
	data.ReplicaCount = req.ReplicaCount
	data.Description = req.Description
	data.UseWorkloadIdentity = req.UseWorkloadIdentity
	data.RemoteUrl = req.RemoteUrl
	data.Command = req.Command
	data.Args = req.Args
	data.MCPClientConfig = req.MCPClientConfig
	data.Authentication = req.Authentication

	// Set defaults for optional fields
	if data.ReplicaCount == 0 {
		data.ReplicaCount = 1
	}
	if data.EnvironmentVariables == nil {
		data.EnvironmentVariables = make(map[string]string)
	}
	if data.Protocol == "" {
		data.Protocol = models.ServerProtocolMCP
	}
	if data.ConnectionType == "" {
		data.ConnectionType = models.ConnectionTypeStreamableHttp
	}
	log.Printf("CreateAdapter: After defaults: %+v", data)
	log.Printf("DEBUG: data.ConnectionType after defaults: %s", data.ConnectionType)

	// Validate name
	log.Printf("CreateAdapter: Validating name: %s", data.Name)
	if !regexp.MustCompile(`^[a-z0-9-]+$`).MatchString(data.Name) {
		log.Printf("CreateAdapter: Name validation failed: %s", data.Name)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name must contain only lowercase letters, numbers, and dashes."})
		return
	}
	log.Printf("CreateAdapter: Name validation passed")

	// Validate based on connection type
	switch data.ConnectionType {
	case models.ConnectionTypeRemoteHttp:
		if data.RemoteUrl == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "RemoteUrl is required for RemoteHttp adapters"})
			return
		}
		// TODO: Validate URL format and allowlist
	case models.ConnectionTypeLocalStdio:
		// Either Command or MCPClientConfig must be provided
		if data.Command == "" && len(data.MCPClientConfig.MCPServers) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Either command or mcpClientConfig is required for LocalStdio adapters"})
			return
		}
		// TODO: Validate command allowlist
	case models.ConnectionTypeSSE, models.ConnectionTypeStreamableHttp:
		if data.ImageName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ImageName is required for K8s adapters"})
			return
		}
	}

	ctx := context.Background()
	user := c.GetString("user") // From auth middleware
	log.Printf("CreateAdapter: User: %s", user)

	// Check if exists
	existing, _ := ms.store.Get(ctx, data.Name)
	if existing != nil {
		log.Printf("CreateAdapter: Adapter already exists: %s", data.Name)
		c.JSON(http.StatusBadRequest, gin.H{"error": "The adapter with the same name already exists."})
		return
	}
	log.Printf("CreateAdapter: Adapter does not exist, proceeding")

	// Create resource
	var resource models.AdapterResource
	resource.Create(data, user, time.Now())

	// Deploy based on connection type
	if data.ConnectionType == models.ConnectionTypeSSE || data.ConnectionType == models.ConnectionTypeStreamableHttp {
		if ms.kubeClient != nil {
			if err := ms.deployAdapter(&data, ctx); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to deploy adapter: %v", err)})
				return
			}
		} else {
			log.Printf("Skipping Kubernetes deployment for adapter %s (running in local mode)", data.Name)
		}
	} else {
		log.Printf("Skipping deployment for non-K8s adapter %s (type: %s)", data.Name, data.ConnectionType)
	}

	// Discover MCP capabilities for supported connection types
	if data.ConnectionType == models.ConnectionTypeRemoteHttp ||
		data.ConnectionType == models.ConnectionTypeStreamableHttp {
		log.Printf("CreateAdapter: Discovering MCP capabilities for adapter %s", data.Name)

		// Wait a moment for the adapter to be ready (especially for deployed adapters)
		if data.ConnectionType == models.ConnectionTypeStreamableHttp {
			time.Sleep(2 * time.Second)
		}

		functionality, err := ms.mcpDiscovery.DiscoverCapabilities(resource)
		if err != nil {
			log.Printf("CreateAdapter: Failed to discover MCP capabilities for %s: %v", data.Name, err)
			// Don't fail the creation, just log the error
		} else {
			resource.MCPFunctionality = functionality
			log.Printf("CreateAdapter: Successfully discovered capabilities for %s", data.Name)
		}
	}

	// Store in memory/Cosmos
	if err := ms.store.UpsertAsync(resource, ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to store adapter: %v", err)})
		return
	}

	c.JSON(http.StatusCreated, resource)
}

// ListAdapters handles GET /adapters
// @Summary List all MCP server adapters
// @Description Returns a list of all MCP server adapters that the user can access.
// @Tags adapters
// @Produce json
// @Success 200 {array} models.AdapterResource
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/adapters [get]
func (ms *ManagementService) ListAdapters(c *gin.Context) {
	ctx := context.Background()
	userID := c.GetString("userId")
	if userID == "" {
		userID = "default-user"
	}
	adapters, err := ms.store.List(ctx, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, adapters)
}

// GetAdapter handles GET /adapters/:name
// @Summary Get an adapter
// @Description Retrieve details of a specific MCP server adapter including discovered capabilities.
// @Tags adapters
// @Produce json
// @Param name path string true "Adapter name"
// @Success 200 {object} models.AdapterResource
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/adapters/{name} [get]
func (ms *ManagementService) GetAdapter(c *gin.Context) {
	name := c.Param("name")
	ctx := context.Background()
	userID := c.GetString("userId")
	if userID == "" {
		userID = "default-user"
	}
	adapter, err := ms.store.Get(ctx, name)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	// Check if adapter belongs to user
	if adapter.CreatedBy != userID {
		c.Status(http.StatusNotFound)
		return
	}

	// Check if MCP functionality needs refresh
	refresh := c.Query("refresh") == "true"
	if refresh && (adapter.ConnectionType == models.ConnectionTypeRemoteHttp ||
		adapter.ConnectionType == models.ConnectionTypeStreamableHttp) {
		log.Printf("GetAdapter: Refreshing MCP capabilities for adapter %s", name)

		functionality, err := ms.mcpDiscovery.DiscoverCapabilities(*adapter)
		if err != nil {
			log.Printf("GetAdapter: Failed to refresh MCP capabilities for %s: %v", name, err)
			// Don't fail the request, just log the error
		} else {
			adapter.MCPFunctionality = functionality
			// Update the stored adapter with refreshed capabilities
			if err := ms.store.UpsertAsync(*adapter, ctx); err != nil {
				log.Printf("GetAdapter: Failed to store refreshed capabilities for %s: %v", name, err)
			}
			log.Printf("GetAdapter: Successfully refreshed capabilities for %s", name)
		}
	}

	c.JSON(http.StatusOK, adapter)
}

// UpdateAdapter handles PUT /adapters/:name
// @Summary Update an adapter
// @Description Update the configuration and deployment of an existing MCP server adapter.
// @Tags adapters
// @Accept json
// @Produce json
// @Param name path string true "Adapter name"
// @Param body body models.AdapterResource true "Updated adapter configuration"
// @Success 200 {object} models.AdapterResource
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/adapters/{name} [put]
func (ms *ManagementService) UpdateAdapter(c *gin.Context) {
	name := c.Param("name")
	var data models.AdapterData
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if strings.ToLower(name) != strings.ToLower(data.Name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Adapter name in URL and body must match."})
		return
	}

	ctx := context.Background()
	userID := c.GetString("userId")
	if userID == "" {
		userID = "default-user"
	}
	existing, err := ms.store.Get(ctx, name)
	if err != nil || existing.CreatedBy != userID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "The adapter does not exist."})
		return
	}

	// Update resource
	var updated models.AdapterResource
	updated.Create(data, existing.CreatedBy, existing.CreatedAt)

	// Check if deployment needs update (only for K8s)
	deploymentUpdated := false
	if (existing.ConnectionType == models.ConnectionTypeSSE || existing.ConnectionType == models.ConnectionTypeStreamableHttp) && ms.needsDeploymentUpdate(*existing, data) {
		if ms.kubeClient != nil {
			if err := ms.deployAdapter(&data, ctx); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to update deployment: %v", err)})
				return
			}
			deploymentUpdated = true
		} else {
			log.Printf("Skipping Kubernetes deployment update for adapter %s (running in local mode)", name)
		}
	}

	// Rediscover MCP capabilities if connection type changed or deployment was updated
	shouldRediscover := false
	if existing.ConnectionType != data.ConnectionType ||
		existing.RemoteUrl != data.RemoteUrl ||
		(data.Authentication != nil && existing.Authentication != data.Authentication) ||
		deploymentUpdated {
		shouldRediscover = true
	}

	if shouldRediscover && (data.ConnectionType == models.ConnectionTypeRemoteHttp ||
		data.ConnectionType == models.ConnectionTypeStreamableHttp) {
		log.Printf("UpdateAdapter: Rediscovering MCP capabilities for adapter %s", name)

		// Wait a moment for the adapter to be ready if deployment was updated
		if deploymentUpdated {
			time.Sleep(2 * time.Second)
		}

		functionality, err := ms.mcpDiscovery.DiscoverCapabilities(updated)
		if err != nil {
			log.Printf("UpdateAdapter: Failed to rediscover MCP capabilities for %s: %v", name, err)
			// Don't fail the update, just log the error
		} else {
			updated.MCPFunctionality = functionality
			log.Printf("UpdateAdapter: Successfully rediscovered capabilities for %s", name)
		}
	} else {
		// Preserve existing functionality if not rediscovering
		updated.MCPFunctionality = existing.MCPFunctionality
	}

	if err := ms.store.UpsertAsync(updated, ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, updated)
}

// DeleteAdapter handles DELETE /adapters/:name
// @Summary Delete an adapter
// @Description Remove an MCP server adapter and its deployment.
// @Tags adapters
// @Produce json
// @Param name path string true "Adapter name"
// @Success 204
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/adapters/{name} [delete]
func (ms *ManagementService) DeleteAdapter(c *gin.Context) {
	name := c.Param("name")
	ctx := context.Background()
	userID := c.GetString("userId")
	if userID == "" {
		userID = "default-user"
	}

	// Check ownership
	adapter, err := ms.store.Get(ctx, name)
	if err != nil || adapter.CreatedBy != userID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "The adapter does not exist."})
		return
	}

	if err := ms.store.Delete(ctx, name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Clean up sessions for this adapter
	if err := ms.sessionStore.DeleteByAdapter(name); err != nil {
		log.Printf("ManagementService: Failed to delete sessions for adapter %s: %v", name, err)
		// Don't fail the request for session cleanup errors
	}

	if adapter.ConnectionType == models.ConnectionTypeSSE || adapter.ConnectionType == models.ConnectionTypeStreamableHttp {
		if err := ms.deleteDeployment(name, ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.Status(http.StatusNoContent)
}

// GetAdapterStatus handles GET /adapters/:name/status
// @Summary Get adapter deployment status
// @Description Check the current deployment status of an MCP server adapter.
// @Tags adapters
// @Produce json
// @Param name path string true "Adapter name"
// @Success 200 {object} models.AdapterStatus
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/adapters/{name}/status [get]
func (ms *ManagementService) GetAdapterStatus(c *gin.Context) {
	name := c.Param("name")
	ctx := context.Background()
	userID := c.GetString("userId")
	if userID == "" {
		userID = "default-user"
	}

	// Get adapter to check type
	adapter, err := ms.store.Get(ctx, name)
	if err != nil || adapter.CreatedBy != userID {
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}

	var status models.AdapterStatus
	if adapter.ConnectionType == models.ConnectionTypeSSE || adapter.ConnectionType == models.ConnectionTypeStreamableHttp {
		status, err = ms.getDeploymentStatus(name, ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else {
		// For non-K8s, return mock status
		status = models.AdapterStatus{ReplicaStatus: "Available"}
	}

	c.JSON(http.StatusOK, status)
}

// Helper methods (simplified implementations)
func (ms *ManagementService) deployAdapter(data *models.AdapterData, ctx context.Context) error {
	if ms.kubeClient == nil {
		return fmt.Errorf("Kubernetes client not available - cannot deploy adapter in local mode")
	}

	// Create StatefulSet
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: data.Name,
			Labels: map[string]string{
				"app":  data.Name,
				"type": "mcp-adapter",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: data.Name + "-service",
			Replicas:    func() *int32 { r := int32(data.ReplicaCount); return &r }(),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": data.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":  data.Name,
						"type": "mcp-adapter",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            data.Name + "-container",
							Image:           data.ImageName + ":" + data.ImageVersion,
							ImagePullPolicy: corev1.PullAlways,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8000,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: ms.envVarsFromMap(data.EnvironmentVariables),
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("250m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
						},
					},
				},
			},
		},
	}

	// Create headless service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: data.Name + "-service",
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Selector: map[string]string{
				"app": data.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Port:       8000,
					TargetPort: intstr.FromInt(8000),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	// Deploy StatefulSet
	if err := ms.kubeClient.UpsertStatefulSet(statefulSet, "adapter", ctx); err != nil {
		return fmt.Errorf("failed to create StatefulSet: %w", err)
	}

	// Deploy Service
	if err := ms.kubeClient.UpsertService(service, "adapter", ctx); err != nil {
		return fmt.Errorf("failed to create Service: %w", err)
	}

	return nil
}

func (ms *ManagementService) envVarsFromMap(envMap map[string]string) []corev1.EnvVar {
	var envVars []corev1.EnvVar
	for k, v := range envMap {
		envVars = append(envVars, corev1.EnvVar{
			Name:  k,
			Value: v,
		})
	}
	return envVars
}

// getPodAddresses returns addresses of healthy pods for an adapter
func (ms *ManagementService) getPodAddresses(adapterName string, ctx context.Context) ([]string, error) {
	podList, err := ms.kubeClient.ListPods("adapter",
		fmt.Sprintf("app=%s,type=mcp-adapter", adapterName),
		"status.phase=Running", ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	var addresses []string
	for _, pod := range podList.Items {
		if ms.isPodReady(&pod) {
			address := fmt.Sprintf("http://%s.%s-service.adapter.svc.cluster.local:8000",
				pod.Name, adapterName)
			addresses = append(addresses, address)
		}
	}
	return addresses, nil
}

// isPodReady checks if a pod is ready
func (ms *ManagementService) isPodReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func (ms *ManagementService) deleteDeployment(name string, ctx context.Context) error {
	if ms.kubeClient == nil {
		// Skip Kubernetes deletion for local development
		log.Printf("Skipping Kubernetes deployment deletion for adapter %s (running in local mode)", name)
		return nil
	}
	return ms.kubeClient.DeleteStatefulSet(name, "adapter", ctx)
}

func (ms *ManagementService) getDeploymentStatus(name string, ctx context.Context) (models.AdapterStatus, error) {
	if ms.kubeClient == nil {
		// Return mock status for local development
		ready := 1
		updated := 1
		available := 1
		return models.AdapterStatus{
			ReadyReplicas:     &ready,
			UpdatedReplicas:   &updated,
			AvailableReplicas: &available,
			Image:             "local-development",
			ReplicaStatus:     "Running (local mode)",
		}, nil
	}

	sts, err := ms.kubeClient.ReadStatefulSet(name, "adapter", ctx)
	if err != nil {
		return models.AdapterStatus{}, fmt.Errorf("failed to get StatefulSet: %w", err)
	}

	status := models.AdapterStatus{
		Image: sts.Spec.Template.Spec.Containers[0].Image,
	}

	// Set replica counts
	ready := int(sts.Status.ReadyReplicas)
	status.ReadyReplicas = &ready

	updated := int(sts.Status.UpdatedReplicas)
	status.UpdatedReplicas = &updated

	available := int(sts.Status.AvailableReplicas)
	status.AvailableReplicas = &available

	// Determine replica status
	if sts.Spec.Replicas != nil {
		if ready == int(*sts.Spec.Replicas) {
			status.ReplicaStatus = "Healthy"
		} else {
			status.ReplicaStatus = fmt.Sprintf("Degraded: %d/%d ready", ready, int(*sts.Spec.Replicas))
		}
	}

	return status, nil
}

func (ms *ManagementService) getDeploymentLogs(name string, ordinal int, ctx context.Context) (string, error) {
	if ms.kubeClient == nil {
		// Return mock logs for local development
		return fmt.Sprintf("Mock logs for adapter %s (running in local development mode)\nStarted at: %s\nStatus: Running\nNo actual container logs available.", name, time.Now().Format(time.RFC3339)), nil
	}

	podName := fmt.Sprintf("%s-%d", name, ordinal)
	return ms.kubeClient.GetContainerLogStream(podName, 1000, "adapter", ctx)
}

func (ms *ManagementService) needsDeploymentUpdate(existing models.AdapterResource, new models.AdapterData) bool {
	// Check if fields like ReplicaCount, ImageName, etc., changed
	return existing.ReplicaCount != new.ReplicaCount || existing.ImageName != new.ImageName || existing.ImageVersion != new.ImageVersion
}
