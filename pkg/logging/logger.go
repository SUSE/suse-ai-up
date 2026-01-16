package logging

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// LogLevel represents different logging levels
type LogLevel int

const (
	INFO LogLevel = iota
	WARN
	ERROR
	SUCCESS
)

// ServiceType represents different service types for colored logging
type ServiceType string

const (
	ServiceProxy     ServiceType = "PROXY"
	ServiceRegistry  ServiceType = "REGISTRY"
	ServiceDiscovery ServiceType = "DISCOVERY"
	ServicePlugins   ServiceType = "PLUGINS"
	ServiceHealth    ServiceType = "HEALTH"
	ServiceAdapter   ServiceType = "ADAPTER"
	ServiceMCP       ServiceType = "MCP"
)

// Color codes for terminal output
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
	colorBold   = "\033[1m"
)

// Logger provides structured, colored logging
type Logger struct {
	service ServiceType
	level   LogLevel
}

// NewLogger creates a new logger for a specific service
func NewLogger(service ServiceType) *Logger {
	return &Logger{
		service: service,
		level:   INFO,
	}
}

// SetLevel sets the minimum log level
func (l *Logger) SetLevel(level LogLevel) {
	l.level = level
}

// Info logs an info message
func (l *Logger) Info(message string, args ...interface{}) {
	l.log(INFO, message, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(message string, args ...interface{}) {
	l.log(WARN, message, args...)
}

// Error logs an error message
func (l *Logger) Error(message string, args ...interface{}) {
	l.log(ERROR, message, args...)
}

// Success logs a success message
func (l *Logger) Success(message string, args ...interface{}) {
	l.log(SUCCESS, message, args...)
}

// log handles the actual logging with colors
func (l *Logger) log(level LogLevel, message string, args ...interface{}) {
	if level < l.level {
		return
	}

	timestamp := time.Now().Format("2006/01/02 15:04:05")
	serviceName := fmt.Sprintf("[%s]", l.service)

	var levelStr, color string
	switch level {
	case INFO:
		levelStr = "INFO"
		color = colorBlue
	case WARN:
		levelStr = "WARN"
		color = colorYellow
	case ERROR:
		levelStr = "ERROR"
		color = colorRed
	case SUCCESS:
		levelStr = "SUCCESS"
		color = colorGreen
	}

	// Format the message
	if len(args) > 0 {
		message = fmt.Sprintf(message, args...)
	}

	// Create the full log line
	logLine := fmt.Sprintf("%s %s%-12s%s %s %s",
		timestamp,
		color,
		serviceName,
		colorReset,
		levelStr,
		message)

	// Output to stdout
	fmt.Println(logLine)
}

// Global loggers for different services
var (
	ProxyLogger     = NewLogger(ServiceProxy)
	RegistryLogger  = NewLogger(ServiceRegistry)
	DiscoveryLogger = NewLogger(ServiceDiscovery)
	PluginsLogger   = NewLogger(ServicePlugins)
	HealthLogger    = NewLogger(ServiceHealth)
	AdapterLogger   = NewLogger(ServiceAdapter)
	MCPLogger       = NewLogger(ServiceMCP)
)

// InitGlobalLoggers initializes all global loggers with appropriate settings
func InitGlobalLoggers() {
	// Set up any global logger configuration here
	// For now, all loggers use INFO level by default
}

// ServiceBanner prints a colored banner for service startup
func ServiceBanner(service ServiceType, message string, port int, tlsPort int) {
	// Create each line with proper padding (76 characters content width)
	serviceText := fmt.Sprintf("%s%-20s%s Service Started Successfully", colorBold+colorCyan, service, colorReset)
	emptyLine := ""
	httpText := fmt.Sprintf("HTTP  Port: %s%d%s", colorGreen, port, colorReset)
	httpsText := fmt.Sprintf("HTTPS Port: %s%d%s", colorGreen, tlsPort, colorReset)

	banner := fmt.Sprintf(`
╔══════════════════════════════════════════════════════════════╗
║ %-73s ║
║ %-60s ║
║ %-69s ║
║ %-69s ║
║ %-60s ║
║ %-60s ║
╚══════════════════════════════════════════════════════════════╝
`, serviceText, emptyLine, httpText, httpsText, emptyLine, message)

	fmt.Print(banner)
}

// ShutdownBanner prints a colored shutdown message
func ShutdownBanner(service ServiceType) {
	message := fmt.Sprintf("%s%s%s service shutting down gracefully...", colorBold+colorYellow, service, colorReset)
	fmt.Println(message)
}

// StartupMessage prints a startup message for a service
func StartupMessage(service ServiceType, message string) {
	logger := getLoggerForService(service)
	logger.Success(message)
}

// ErrorMessage prints an error message for a service
func ErrorMessage(service ServiceType, message string, err error) {
	logger := getLoggerForService(service)
	if err != nil {
		logger.Error("%s: %v", message, err)
	} else {
		logger.Error(message)
	}
}

// getLoggerForService returns the appropriate logger for a service type
func getLoggerForService(service ServiceType) *Logger {
	switch service {
	case ServiceProxy:
		return ProxyLogger
	case ServiceRegistry:
		return RegistryLogger
	case ServiceDiscovery:
		return DiscoveryLogger
	case ServicePlugins:
		return PluginsLogger
	case ServiceHealth:
		return HealthLogger
	case ServiceAdapter:
		return AdapterLogger
	case ServiceMCP:
		return MCPLogger
	default:
		return NewLogger(service)
	}
}

// DisableColors disables ANSI color codes (useful for log files)
func DisableColors() {
	// This would require modifying the color constants to empty strings
	// For now, colors are always enabled
}

// EnableFileLogging enables logging to a file in addition to stdout
func EnableFileLogging(filename string) error {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return err
	}

	// Create a multi-writer that writes to both stdout and file
	multiWriter := &multiWriter{
		writers: []interface{ Write([]byte) (int, error) }{os.Stdout, file},
	}

	// Redirect log package to use our multi-writer (for compatibility)
	log.SetOutput(multiWriter)

	return nil
}

// multiWriter writes to multiple destinations
type multiWriter struct {
	writers []interface{ Write([]byte) (int, error) }
}

func (mw *multiWriter) Write(p []byte) (n int, err error) {
	for _, writer := range mw.writers {
		// Strip ANSI color codes for file output
		cleanData := stripANSI(string(p))
		writer.Write([]byte(cleanData))
	}
	return len(p), nil
}

// stripANSI removes ANSI color codes from a string
func stripANSI(str string) string {
	// Simple ANSI code removal - in a real implementation you'd want a more robust solution
	result := str
	for _, code := range []string{"\033[0m", "\033[31m", "\033[32m", "\033[33m", "\033[34m", "\033[35m", "\033[36m", "\033[37m", "\033[1m"} {
		result = strings.ReplaceAll(result, code, "")
	}
	return result
}
