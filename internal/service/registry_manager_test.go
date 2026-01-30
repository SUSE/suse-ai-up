package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"suse-ai-up/pkg/models"
)

// MockMCPServerStore is a mock implementation of MCPServerStore
type MockMCPServerStore struct {
	mock.Mock
}

func (m *MockMCPServerStore) CreateMCPServer(server *models.MCPServer) error {
	args := m.Called(server)
	return args.Error(0)
}

func (m *MockMCPServerStore) GetMCPServer(id string) (*models.MCPServer, error) {
	args := m.Called(id)
	return args.Get(0).(*models.MCPServer), args.Error(1)
}

func (m *MockMCPServerStore) UpdateMCPServer(id string, updated *models.MCPServer) error {
	args := m.Called(id, updated)
	return args.Error(0)
}

func (m *MockMCPServerStore) DeleteMCPServer(id string) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockMCPServerStore) ListMCPServers() []*models.MCPServer {
	args := m.Called()
	return args.Get(0).([]*models.MCPServer)
}

// TestLoadFromCustomSource_PublicURL tests loading from a public URL without authentication
func TestLoadFromCustomSource_PublicURL(t *testing.T) {
	// Mock HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "no-cache, no-store, must-revalidate", r.Header.Get("Cache-Control"))
		assert.Equal(t, "no-cache", r.Header.Get("Pragma"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id": "test-server", "name": "Test Server", "description": "A test server", "version": "1.0", "packages": []}]`))
	}))
	defer ts.Close()

	mockStore := new(MockMCPServerStore)
	mockStore.On("CreateMCPServer", mock.AnythingOfType("*models.MCPServer")).Return(nil)

	rm := NewRegistryManager(mockStore, false, 0, nil, nil) // k8sClient is nil for this test

	sourceConfig := models.RegistrySourceConfig{
		URL: ts.URL,
	}

	err := rm.LoadFromCustomSource(sourceConfig)
	assert.NoError(t, err)

	mockStore.AssertExpectations(t)
}

// TestLoadFromCustomSource_BearerAuth tests loading from a URL with Bearer authentication
func TestLoadFromCustomSource_BearerAuth(t *testing.T) {
	const secretName = "test-secret"
	const secretKey = "token"
	const authToken = "my-bearer-token"

	// Mock Kubernetes client
	fakeK8sClient := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: "default",
		},
		Data: map[string][]byte{
			secretKey: []byte(authToken),
		},
	})

	// Mock HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer "+authToken, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id": "auth-server", "name": "Auth Server", "description": "Auth test", "version": "1.0", "packages": []}]`))
	}))
	defer ts.Close()

	mockStore := new(MockMCPServerStore)
	mockStore.On("CreateMCPServer", mock.AnythingOfType("*models.MCPServer")).Return(nil)

	rm := NewRegistryManager(mockStore, false, 0, nil, fakeK8sClient)

	sourceConfig := models.RegistrySourceConfig{
		URL: ts.URL,
		Auth: &models.AuthConfig{
			SecretName: secretName,
			SecretKey:  secretKey,
			Type:       "bearer",
		},
	}

	err := rm.LoadFromCustomSource(sourceConfig)
	assert.NoError(t, err)

	mockStore.AssertExpectations(t)
}

// TestLoadFromCustomSource_BasicAuth tests loading from a URL with Basic authentication
func TestLoadFromCustomSource_BasicAuth(t *testing.T) {
	const secretName = "test-secret-basic"
	const secretKey = "credentials"              // format "username:password" base64 encoded
	const authToken = "dXNlcm5hbWU6cGFzc3dvcmQ=" // base64 encoded "username:password"

	// Mock Kubernetes client
	fakeK8sClient := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: "default",
		},
		Data: map[string][]byte{
			secretKey: []byte(authToken),
		},
	})

	// Mock HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Basic "+authToken, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id": "basic-server", "name": "Basic Server", "description": "Basic auth test", "version": "1.0", "packages": []}]`))
	}))
	defer ts.Close()

	mockStore := new(MockMCPServerStore)
	mockStore.On("CreateMCPServer", mock.AnythingOfType("*models.MCPServer")).Return(nil)

	rm := NewRegistryManager(mockStore, false, 0, nil, fakeK8sClient)

	sourceConfig := models.RegistrySourceConfig{
		URL: ts.URL,
		Auth: &models.AuthConfig{
			SecretName: secretName,
			SecretKey:  secretKey,
			Type:       "basic",
		},
	}

	err := rm.LoadFromCustomSource(sourceConfig)
	assert.NoError(t, err)

	mockStore.AssertExpectations(t)
}

// TestSyncAllSources_CustomSources tests SyncAllSources with custom sources from RegistryManager.customSources
func TestSyncAllSources_CustomSources(t *testing.T) {
	// Mock HTTP server
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id": "cs-server1", "name": "CS Server 1", "description": "CS test 1", "version": "1.0", "packages": []}]`))
	}))
	defer ts1.Close()

	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id": "cs-server2", "name": "CS Server 2", "description": "CS test 2", "version": "1.0", "packages": []}]`))
	}))
	defer ts2.Close()

	mockStore := new(MockMCPServerStore)
	mockStore.On("CreateMCPServer", mock.AnythingOfType("*models.MCPServer")).Return(nil)

	customSources := []models.RegistrySourceConfig{
		{URL: ts1.URL},
		{URL: ts2.URL},
	}

	rm := NewRegistryManager(mockStore, false, 0, customSources, nil)

	err := rm.SyncAllSources(context.Background())
	assert.NoError(t, err)

	mockStore.AssertExpectations(t)
	mockStore.AssertNumberOfCalls(t, "CreateMCPServer", 2)
}

// TestSyncAllSources_FileCustomSources tests SyncAllSources with MCP_CUSTOM_REGISTRY_SOURCES_PATH env var
func TestSyncAllSources_FileCustomSources(t *testing.T) {
	// Create a temporary file for custom sources
	tempFile, err := os.CreateTemp("", "custom_registry_sources_*.json")
	assert.NoError(t, err)
	defer os.Remove(tempFile.Name())

	// Mock HTTP server
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id": "file-server1", "name": "File Server 1", "description": "File test 1", "version": "1.0", "packages": []}]`))
	}))
	defer ts1.Close()

	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id": "file-server2", "name": "File Server 2", "description": "File test 2", "version": "1.0", "packages": []}]`))
	}))
	defer ts2.Close()

	fileSources := []models.RegistrySourceConfig{
		{URL: ts1.URL},
		{URL: ts2.URL},
	}
	fileContent, err := json.Marshal(fileSources)
	assert.NoError(t, err)
	_, err = tempFile.Write(fileContent)
	assert.NoError(t, err)
	tempFile.Close()

	// Set environment variable
	os.Setenv("MCP_CUSTOM_REGISTRY_SOURCES_PATH", tempFile.Name())
	defer os.Unsetenv("MCP_CUSTOM_REGISTRY_SOURCES_PATH")

	mockStore := new(MockMCPServerStore)
	mockStore.On("CreateMCPServer", mock.AnythingOfType("*models.MCPServer")).Return(nil)

	rm := NewRegistryManager(mockStore, false, 0, nil, nil) // No initial custom sources, will load from file

	err = rm.SyncAllSources(context.Background())
	assert.NoError(t, err)

	mockStore.AssertExpectations(t)
	mockStore.AssertNumberOfCalls(t, "CreateMCPServer", 2)
}

// TestSyncAllSources_FileCustomSources_WithAuth tests SyncAllSources with file-based custom sources and authentication
func TestSyncAllSources_FileCustomSources_WithAuth(t *testing.T) {
	// Create a temporary file for custom sources
	tempFile, err := os.CreateTemp("", "custom_registry_sources_auth_*.json")
	assert.NoError(t, err)
	defer os.Remove(tempFile.Name())

	const secretName = "file-test-secret"
	const secretKey = "token"
	const authToken = "file-bearer-token"

	// Mock Kubernetes client
	fakeK8sClient := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: "default",
		},
		Data: map[string][]byte{
			secretKey: []byte(authToken),
		},
	})

	// Mock HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer "+authToken, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id": "file-auth-server", "name": "File Auth Server", "description": "File auth test", "version": "1.0", "packages": []}]`))
	}))
	defer ts.Close()

	fileSources := []models.RegistrySourceConfig{
		{
			URL: ts.URL,
			Auth: &models.AuthConfig{
				SecretName: secretName,
				SecretKey:  secretKey,
				Type:       "bearer",
			},
		},
	}
	fileContent, err := json.Marshal(fileSources)
	assert.NoError(t, err)
	_, err = tempFile.Write(fileContent)
	assert.NoError(t, err)
	tempFile.Close()

	// Set environment variable
	os.Setenv("POD_NAMESPACE", "default") // Required for K8s client to find secret
	os.Setenv("MCP_CUSTOM_REGISTRY_SOURCES_PATH", tempFile.Name())
	defer os.Unsetenv("POD_NAMESPACE")
	defer os.Unsetenv("MCP_CUSTOM_REGISTRY_SOURCES_PATH")

	mockStore := new(MockMCPServerStore)
	mockStore.On("CreateMCPServer", mock.AnythingOfType("*models.MCPServer")).Return(nil)

	rm := NewRegistryManager(mockStore, false, 0, nil, fakeK8sClient)

	err = rm.SyncAllSources(context.Background())
	assert.NoError(t, err)

	mockStore.AssertExpectations(t)
	mockStore.AssertNumberOfCalls(t, "CreateMCPServer", 1)
}
