package middleware

import (
	"net/http"
	"net/url"
	"os"
	"strings"
)

// CORSMiddleware handles CORS headers for cross-origin requests
func CORSMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get allowed origins from environment variable
		allowedOriginsStr := os.Getenv("CORS_ALLOWED_ORIGINS")
		if allowedOriginsStr == "" {
			// Default for development - allow localhost and any HTTPS origins for public access
			allowedOriginsStr = "http://localhost:3000,http://localhost:8080,http://127.0.0.1:3000,*"
		}

		allowedOrigins := strings.Split(allowedOriginsStr, ",")
		origin := r.Header.Get("Origin")

		// Check if origin is allowed
		allowOrigin := isOriginAllowed(origin, allowedOrigins)

		// Set CORS headers
		if allowOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-User-ID")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		// Handle preflight OPTIONS request
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// isOriginAllowed checks if the given origin is allowed based on the configured allowed origins
func isOriginAllowed(origin string, allowedOrigins []string) string {
	if origin == "" {
		return ""
	}

	// Parse the origin URL
	originURL, err := url.Parse(origin)
	if err != nil {
		return ""
	}

	// Check for wildcard "*" first - allows all origins
	for _, allowed := range allowedOrigins {
		allowed = strings.TrimSpace(allowed)
		if allowed == "*" {
			return origin
		}
	}

	// Check exact matches
	for _, allowed := range allowedOrigins {
		if strings.TrimSpace(allowed) == origin {
			return origin
		}
	}

	// For public IP access, allow origins that are valid HTTP/HTTPS URLs
	// This allows webapps running on public IPs or load balancers to access the API
	if originURL.Scheme == "http" || originURL.Scheme == "https" {
		// Allow any valid HTTP/HTTPS origin for public access
		// This is more permissive but necessary for load balancer and public IP scenarios
		return origin
	}

	return ""
}
