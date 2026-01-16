package middleware

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestResponseLogger provides structured HTTP request/response logging
type RequestResponseLogger struct {
	// IncludeRequestBody determines if request bodies should be logged
	IncludeRequestBody bool
	// IncludeResponseBody determines if response bodies should be logged
	IncludeResponseBody bool
	// MaxBodySize limits the size of logged bodies
	MaxBodySize int64
}

// NewRequestResponseLogger creates a new request/response logger middleware
func NewRequestResponseLogger() *RequestResponseLogger {
	return &RequestResponseLogger{
		IncludeRequestBody:  false, // Disabled by default for security
		IncludeResponseBody: false, // Disabled by default for performance
		MaxBodySize:         4096,  // 4KB limit
	}
}

// GinMiddleware returns a Gin middleware function for request/response logging
func (l *RequestResponseLogger) GinMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Generate correlation ID
		correlationID := uuid.New().String()
		c.Set("correlation_id", correlationID)

		// Add correlation ID to response header
		c.Header("X-Correlation-ID", correlationID)

		start := time.Now()

		// Log request
		l.logRequest(c, correlationID)

		// Capture response
		responseWriter := &responseCaptureWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
		}
		c.Writer = responseWriter

		// Process request
		c.Next()

		// Calculate duration
		duration := time.Since(start)

		// Log response
		l.logResponse(c, correlationID, duration, responseWriter)
	}
}

func (l *RequestResponseLogger) logRequest(c *gin.Context, correlationID string) {
	var requestBody string
	if l.IncludeRequestBody && c.Request.Body != nil {
		bodyBytes, err := io.ReadAll(io.LimitReader(c.Request.Body, l.MaxBodySize))
		if err == nil {
			requestBody = string(bodyBytes)
			// Restore the body for further processing
			c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
	}

	logEntry := fmt.Sprintf("[REQUEST] %s %s %s | CorrelationID: %s | User-Agent: %s | IP: %s",
		c.Request.Method,
		c.Request.URL.Path,
		c.Request.URL.RawQuery,
		correlationID,
		c.GetHeader("User-Agent"),
		c.ClientIP(),
	)

	if requestBody != "" {
		logEntry += fmt.Sprintf(" | Body: %s", truncateString(requestBody, 200))
	}

	log.Println(logEntry)
}

func (l *RequestResponseLogger) logResponse(c *gin.Context, correlationID string, duration time.Duration, writer *responseCaptureWriter) {
	var responseBody string
	if l.IncludeResponseBody {
		responseBody = writer.body.String()
	}

	statusCode := c.Writer.Status()
	statusText := http.StatusText(statusCode)

	logEntry := fmt.Sprintf("[RESPONSE] %d %s | CorrelationID: %s | Duration: %v | Content-Type: %s | Content-Length: %d",
		statusCode,
		statusText,
		correlationID,
		duration,
		c.GetHeader("Content-Type"),
		c.Writer.Size(),
	)

	if responseBody != "" {
		logEntry += fmt.Sprintf(" | Body: %s", truncateString(responseBody, 200))
	}

	// Color code based on status
	if statusCode >= 500 {
		logEntry = fmt.Sprintf("\033[31m%s\033[0m", logEntry) // Red for server errors
	} else if statusCode >= 400 {
		logEntry = fmt.Sprintf("\033[33m%s\033[0m", logEntry) // Yellow for client errors
	} else if statusCode >= 200 {
		logEntry = fmt.Sprintf("\033[32m%s\033[0m", logEntry) // Green for success
	}

	log.Println(logEntry)
}

// responseCaptureWriter captures the response body while still writing to the original writer
type responseCaptureWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *responseCaptureWriter) Write(data []byte) (int, error) {
	// Write to the original response writer
	n, err := w.ResponseWriter.Write(data)
	if err != nil {
		return n, err
	}

	// Also capture in our buffer
	w.body.Write(data[:n])
	return n, err
}

func (w *responseCaptureWriter) WriteString(data string) (int, error) {
	// Write to the original response writer
	n, err := w.ResponseWriter.WriteString(data)
	if err != nil {
		return n, err
	}

	// Also capture in our buffer
	w.body.WriteString(data[:n])
	return n, err
}

// truncateString truncates a string to the specified length with ellipsis
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	// Try to truncate at word boundary if possible
	if maxLen > 3 {
		truncated := s[:maxLen-3]
		if lastSpace := strings.LastIndex(truncated, " "); lastSpace > maxLen/2 {
			return truncated[:lastSpace] + "..."
		}
	}

	return s[:maxLen-3] + "..."
}
