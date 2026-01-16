package middleware

import (
	"net/http"
	"os"
	"strings"
)

// CreateAuthenticatedClient creates an HTTP client with API key authentication
func CreateAuthenticatedClient() *http.Client {
	return &http.Client{}
}

// AddAPIKeyAuth adds API key authentication to an HTTP request
func AddAPIKeyAuth(req *http.Request) {
	apiKey := os.Getenv("SERVICE_API_KEYS")
	if apiKey != "" {
		// Use the first key if multiple are specified
		keys := strings.Split(apiKey, ",")
		if len(keys) > 0 {
			req.Header.Set("X-API-Key", strings.TrimSpace(keys[0]))
		}
	}
}
