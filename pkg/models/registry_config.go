package models

// AuthConfig defines authentication details for a registry source.
// It currently supports token-based authentication via Kubernetes secrets.
type AuthConfig struct {
	// SecretName refers to the name of the Kubernetes Secret containing the authentication token.
	SecretName string `json:"secretName,omitempty"`
	// SecretKey refers to the key within the Kubernetes Secret where the token value is stored.
	SecretKey string `json:"secretKey,omitempty"`
	// Type specifies the authentication type, e.g., "bearer", "basic".
	Type string `json:"type,omitempty"`
}

// RegistrySourceConfig defines the configuration for a single remote registry source.
type RegistrySourceConfig struct {
	// URL is the HTTP/HTTPS URL of the remote registry.
	URL string `json:"url"`
	// Auth contains optional authentication details for accessing the URL.
	Auth *AuthConfig `json:"auth,omitempty"`
}
