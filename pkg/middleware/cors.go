package middleware

import (
	"net/http"
	"os"
	"strings"
)

// CORSMiddleware handles CORS headers for cross-origin requests
func CORSMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get allowed origins from environment variable
		allowedOriginsStr := os.Getenv("CORS_ALLOWED_ORIGINS")
		if allowedOriginsStr == "" {
			// Default for development
			allowedOriginsStr = "http://localhost:3000,http://localhost:8080,http://127.0.0.1:3000"
		}

		allowedOrigins := strings.Split(allowedOriginsStr, ",")
		origin := r.Header.Get("Origin")

		// Check if origin is allowed
		allowOrigin := ""
		for _, allowed := range allowedOrigins {
			if strings.TrimSpace(allowed) == origin {
				allowOrigin = origin
				break
			}
		}

		// Set CORS headers
		if allowOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		// Handle preflight OPTIONS request
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}
