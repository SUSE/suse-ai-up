package middleware

import (
	"net/http"
	"os"
)

// CreateAuthenticatedClient creates an HTTP client with API key authentication
func CreateAuthenticatedClient() *http.Client {
	return &http.Client{}
}

// AddAPIKeyAuth adds API key authentication to an HTTP request
func AddAPIKeyAuth(req *http.Request) {
	apiKey := os.Getenv("SERVICE_API_KEY")
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
}
