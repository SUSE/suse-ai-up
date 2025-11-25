package handlers

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"suse-ai-up/pkg/plugins"
)

// PluginHandler handles plugin service registration and management
type PluginHandler struct {
	serviceManager *plugins.ServiceManager
}

// NewPluginHandler creates a new plugin handler
func NewPluginHandler(serviceManager *plugins.ServiceManager) *PluginHandler {
	return &PluginHandler{
		serviceManager: serviceManager,
	}
}

// RegisterServiceRequest represents a service registration request
type RegisterServiceRequest struct {
	ServiceID    string                      `json:"service_id" binding:"required"`
	ServiceType  string                      `json:"service_type" binding:"required"`
	ServiceURL   string                      `json:"service_url" binding:"required"`
	Version      string                      `json:"version"`
	Capabilities []plugins.ServiceCapability `json:"capabilities"`
}

// RegisterServiceResponse represents a service registration response
type RegisterServiceResponse struct {
	Message   string `json:"message"`
	ServiceID string `json:"service_id"`
}

// RegisterService handles POST /api/v1/plugins/register
// @Summary Register a plugin service
// @Description Register a new plugin service with the proxy
// @Tags plugins
// @Accept json
// @Produce json
// @Param request body RegisterServiceRequest true "Service registration request"
// @Success 201 {object} RegisterServiceResponse
// @Failure 400 {object} PluginErrorResponse
// @Failure 409 {object} PluginErrorResponse
// @Failure 500 {object} PluginErrorResponse
// @Router /api/v1/plugins/register [post]
func (h *PluginHandler) RegisterService(c *gin.Context) {
	var req RegisterServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("Plugin registration: Invalid request body: %v", err)
		c.JSON(http.StatusBadRequest, PluginErrorResponse{Error: "Invalid request body", Details: err.Error()})
		return
	}

	log.Printf("Plugin registration: Registering service %s of type %s at %s",
		req.ServiceID, req.ServiceType, req.ServiceURL)

	// Convert service type string to ServiceType (allow any type)
	serviceType := plugins.ServiceType(req.ServiceType)

	// Create service registration
	registration := &plugins.ServiceRegistration{
		ServiceID:    req.ServiceID,
		ServiceType:  serviceType,
		ServiceURL:   req.ServiceURL,
		Capabilities: req.Capabilities,
		Version:      req.Version,
	}

	// Register the service
	if err := h.serviceManager.RegisterService(registration); err != nil {
		log.Printf("Plugin registration: Failed to register service %s: %v", req.ServiceID, err)
		c.JSON(http.StatusConflict, PluginErrorResponse{Error: "Failed to register service", Details: err.Error()})
		return
	}

	log.Printf("Plugin registration: Successfully registered service %s", req.ServiceID)

	response := RegisterServiceResponse{
		Message:   "Service registered successfully",
		ServiceID: req.ServiceID,
	}

	c.JSON(http.StatusCreated, response)
}

// UnregisterService handles DELETE /api/v1/plugins/register/{serviceId}
// @Summary Unregister a plugin service
// @Description Remove a plugin service registration
// @Tags plugins
// @Param serviceId path string true "Service ID"
// @Success 204 "No Content"
// @Failure 404 {object} PluginErrorResponse
// @Failure 500 {object} PluginErrorResponse
// @Router /api/v1/plugins/register/{serviceId} [delete]
func (h *PluginHandler) UnregisterService(c *gin.Context) {
	serviceID := c.Param("serviceId")

	log.Printf("Plugin registration: Unregistering service %s", serviceID)

	if err := h.serviceManager.UnregisterService(serviceID); err != nil {
		log.Printf("Plugin registration: Failed to unregister service %s: %v", serviceID, err)
		c.JSON(http.StatusNotFound, PluginErrorResponse{Error: "Service not found", Details: err.Error()})
		return
	}

	log.Printf("Plugin registration: Successfully unregistered service %s", serviceID)
	c.Status(http.StatusNoContent)
}

// ListServices handles GET /api/v1/plugins/services
// @Summary List all registered plugin services
// @Description Get a list of all registered plugin services
// @Tags plugins
// @Produce json
// @Success 200 {array} plugins.ServiceRegistration
// @Router /api/v1/plugins/services [get]
func (h *PluginHandler) ListServices(c *gin.Context) {
	services := h.serviceManager.GetAllServices()
	c.JSON(http.StatusOK, services)
}

// GetService handles GET /api/v1/plugins/services/{serviceId}
// @Summary Get service details
// @Description Get details of a specific plugin service
// @Tags plugins
// @Produce json
// @Param serviceId path string true "Service ID"
// @Success 200 {object} plugins.ServiceRegistration
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/plugins/services/{serviceId} [get]
func (h *PluginHandler) GetService(c *gin.Context) {
	serviceID := c.Param("serviceId")

	service, exists := h.serviceManager.GetService(serviceID)
	if !exists {
		c.JSON(http.StatusNotFound, PluginErrorResponse{Error: "Service not found", Details: fmt.Sprintf("Service '%s' not found", serviceID)})
		return
	}

	c.JSON(http.StatusOK, service)
}

// ListServicesByType handles GET /api/v1/plugins/services/type/{serviceType}
// @Summary List services by type
// @Description Get all services of a specific type
// @Tags plugins
// @Produce json
// @Param serviceType path string true "Service type (smartagents or registry)"
// @Success 200 {array} plugins.ServiceRegistration
// @Failure 400 {object} PluginErrorResponse
// @Router /api/v1/plugins/services/type/{serviceType} [get]
func (h *PluginHandler) ListServicesByType(c *gin.Context) {
	serviceTypeStr := c.Param("serviceType")

	var serviceType plugins.ServiceType
	// Allow any service type
	serviceType = plugins.ServiceType(serviceTypeStr)

	services := h.serviceManager.GetServicesByType(serviceType)
	c.JSON(http.StatusOK, services)
}

// GetServiceHealth handles GET /api/v1/plugins/services/{serviceId}/health
// @Summary Get service health
// @Description Get the health status of a specific plugin service
// @Tags plugins
// @Produce json
// @Param serviceId path string true "Service ID"
// @Success 200 {object} plugins.ServiceHealth
// @Failure 404 {object} PluginErrorResponse
// @Router /api/v1/plugins/services/{serviceId}/health [get]
func (h *PluginHandler) GetServiceHealth(c *gin.Context) {
	serviceID := c.Param("serviceId")

	health, exists := h.serviceManager.GetServiceHealth(serviceID)
	if !exists {
		c.JSON(http.StatusNotFound, PluginErrorResponse{Error: "Service not found", Details: fmt.Sprintf("Service '%s' not found", serviceID)})
		return
	}

	c.JSON(http.StatusOK, health)
}

// PluginErrorResponse represents a plugin error response
type PluginErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}
