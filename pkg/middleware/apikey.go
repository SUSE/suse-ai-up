package middleware

import (
	"net/http"
	"os"
	"strings"
)

// APIKeyAuthMiddleware validates API key for inter-service requests
func APIKeyAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get valid API keys from environment variable
		validKeysStr := os.Getenv("SERVICE_API_KEYS")
		if validKeysStr == "" {
			// Skip authentication if no keys configured (for development)
			next(w, r)
			return
		}

		// Get API key from header
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			http.Error(w, "Missing API key", http.StatusUnauthorized)
			return
		}

		// Check if API key is valid
		validKeys := strings.Split(validKeysStr, ",")
		valid := false
		for _, key := range validKeys {
			if strings.TrimSpace(key) == apiKey {
				valid = true
				break
			}
		}

		if !valid {
			http.Error(w, "Invalid API key", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}
