package service

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"suse-ai-up/pkg/plugins"
)

// PluginRegistrationService handles plugin service registration
type PluginRegistrationService struct {
	serviceManager plugins.PluginServiceManager
}

// NewPluginRegistrationService creates a new plugin registration service
func NewPluginRegistrationService(serviceManager plugins.PluginServiceManager) *PluginRegistrationService {
	return &PluginRegistrationService{
		serviceManager: serviceManager,
	}
}

// RegisterService handles POST /plugins/register
// @Summary Register a plugin service
// @Description Register a plugin service with the proxy
// @Tags plugins
// @Accept json
// @Produce json
// @Param registration body plugins.ServiceRegistration true "Service registration data"
// @Success 201 {object} plugins.ServiceRegistration
// @Failure 400 {string} string "Bad Request"
// @Failure 409 {string} string "Conflict"
// @Router /plugins/register [post]
func (prs *PluginRegistrationService) RegisterService(c *gin.Context) {
	log.Printf("DEBUG: RegisterService called")
	var registration plugins.ServiceRegistration
	if err := c.ShouldBindJSON(&registration); err != nil {
		log.Printf("Failed to parse service registration: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid registration data"})
		return
	}
	log.Printf("DEBUG: Parsed registration: %+v", registration)
	log.Printf("DEBUG: Parsed registration: %+v", registration)

	// Validate registration
	if registration.ServiceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Service ID is required"})
		return
	}

	if registration.ServiceType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Service type is required"})
		return
	}

	if registration.ServiceURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Service URL is required"})
		return
	}

	// Register the service
	if err := prs.serviceManager.RegisterService(&registration); err != nil {
		log.Printf("Failed to register service %s: %v", registration.ServiceID, err)
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	log.Printf("Successfully registered plugin service: %s (%s)",
		registration.ServiceID, registration.ServiceType)

	c.JSON(http.StatusCreated, registration)
}

// UnregisterService handles DELETE /plugins/register/{serviceId}
// @Summary Unregister a plugin service
// @Description Remove a plugin service registration
// @Tags plugins
// @Param serviceId path string true "Service ID"
// @Success 204 "No Content"
// @Failure 404 {string} string "Not Found"
// @Router /plugins/register/{serviceId} [delete]
func (prs *PluginRegistrationService) UnregisterService(c *gin.Context) {
	serviceID := c.Param("serviceId")

	if err := prs.serviceManager.UnregisterService(serviceID); err != nil {
		log.Printf("Failed to unregister service %s: %v", serviceID, err)
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	log.Printf("Successfully unregistered plugin service: %s", serviceID)
	c.Status(http.StatusNoContent)
}

// ListServices handles GET /plugins/services
// @Summary List registered plugin services
// @Description Get a list of all registered plugin services
// @Tags plugins
// @Produce json
// @Success 200 {array} plugins.ServiceRegistration
// @Router /plugins/services [get]
func (prs *PluginRegistrationService) ListServices(c *gin.Context) {
	services := prs.serviceManager.GetAllServices()
	c.JSON(http.StatusOK, services)
}

// GetService handles GET /plugins/services/{serviceId}
// @Summary Get service details
// @Description Get details of a specific plugin service
// @Tags plugins
// @Produce json
// @Param serviceId path string true "Service ID"
// @Success 200 {object} plugins.ServiceRegistration
// @Failure 404 {string} string "Not Found"
// @Router /plugins/services/{serviceId} [get]
func (prs *PluginRegistrationService) GetService(c *gin.Context) {
	serviceID := c.Param("serviceId")

	service, exists := prs.serviceManager.GetService(serviceID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Service not found"})
		return
	}

	c.JSON(http.StatusOK, service)
}

// GetServiceHealth handles GET /plugins/services/{serviceId}/health
// @Summary Get service health
// @Description Get health status of a specific plugin service
// @Tags plugins
// @Produce json
// @Param serviceId path string true "Service ID"
// @Success 200 {object} plugins.ServiceHealth
// @Failure 404 {string} string "Not Found"
// @Router /plugins/services/{serviceId}/health [get]
func (prs *PluginRegistrationService) GetServiceHealth(c *gin.Context) {
	serviceID := c.Param("serviceId")

	health, exists := prs.serviceManager.GetServiceHealth(serviceID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Service not found"})
		return
	}

	c.JSON(http.StatusOK, health)
}

// ListServicesByType handles GET /plugins/services/type/{serviceType}
// @Summary List services by type
// @Description Get all services of a specific type
// @Tags plugins
// @Produce json
// @Param serviceType path string true "Service type (smartagents, registry)"
// @Success 200 {array} plugins.ServiceRegistration
// @Router /plugins/services/type/{serviceType} [get]
func (prs *PluginRegistrationService) ListServicesByType(c *gin.Context) {
	serviceTypeStr := c.Param("serviceType")

	var serviceType plugins.ServiceType
	switch serviceTypeStr {
	case "smartagents":
		serviceType = plugins.ServiceTypeSmartAgents
	case "registry":
		serviceType = plugins.ServiceTypeRegistry

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid service type"})
		return
	}

	services := prs.serviceManager.GetServicesByType(serviceType)
	c.JSON(http.StatusOK, services)
}
