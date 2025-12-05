package discovery

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/middleware"
	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/scanner"
)

// Service represents the discovery service
type Service struct {
	config      *Config
	server      *http.Server
	store       clients.ScanStore
	scanner     *scanner.NetworkScanner
	shutdownCh  chan struct{}
	mu          sync.RWMutex
	activeScans map[string]*ScanJob
}

// Config holds discovery service configuration
type Config struct {
	Port           int           `json:"port"`
	TLSPort        int           `json:"tls_port"`
	DefaultTimeout time.Duration `json:"default_timeout"`
	MaxConcurrency int           `json:"max_concurrency"`
	ExcludeProxy   bool          `json:"exclude_proxy"`
	AutoTLS        bool          `json:"auto_tls"`
	CertFile       string        `json:"cert_file"`
	KeyFile        string        `json:"key_file"`
}

// ScanJob represents an active scan operation
type ScanJob struct {
	ID        string
	Config    models.ScanConfig
	StartTime time.Time
	Status    string
	Results   []models.DiscoveredServer
	Error     string
	cancel    context.CancelFunc
}

// NewService creates a new discovery service
func NewService(config *Config) *Service {
	if config.DefaultTimeout == 0 {
		config.DefaultTimeout = 30 * time.Second
	}
	if config.MaxConcurrency == 0 {
		config.MaxConcurrency = 10
	}

	service := &Service{
		config:      config,
		store:       clients.NewInMemoryScanStore(),
		activeScans: make(map[string]*ScanJob),
		shutdownCh:  make(chan struct{}),
	}

	return service
}

// Start starts the discovery service
func (s *Service) Start() error {
	log.Printf("Starting MCP Discovery service on port %d", s.config.Port)

	// Setup HTTP routes with CORS middleware
	mux := http.NewServeMux()
	mux.HandleFunc("/health", middleware.CORSMiddleware(s.handleHealth))
	mux.HandleFunc("/api/v1/scan", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(s.handleStartScan)))
	mux.HandleFunc("/api/v1/scan/", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(s.handleGetScanStatus)))
	mux.HandleFunc("/api/v1/servers", middleware.CORSMiddleware(middleware.APIKeyAuthMiddleware(s.handleListServers)))

	// Start HTTP server
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.Port),
		Handler: mux,
	}

	go func() {
		log.Printf("MCP Discovery HTTP server listening on port %d", s.config.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Start HTTPS server if TLS is configured
	if s.config.TLSPort > 0 {
		tlsConfig := &tls.Config{
			ServerName: "localhost",
		}

		// Try to load certificates
		if s.config.CertFile != "" && s.config.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(s.config.CertFile, s.config.KeyFile)
			if err != nil {
				log.Printf("Failed to load TLS certificates: %v", err)
				if !s.config.AutoTLS {
					return fmt.Errorf("TLS certificates required but failed to load: %w", err)
				}
			} else {
				tlsConfig.Certificates = []tls.Certificate{cert}
			}
		}

		// Generate self-signed certificate if AutoTLS is enabled and no certs loaded
		if s.config.AutoTLS && len(tlsConfig.Certificates) == 0 {
			cert, err := s.generateSelfSignedCert()
			if err != nil {
				return fmt.Errorf("failed to generate self-signed certificate: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{*cert}
			log.Printf("Generated self-signed TLS certificate for discovery service")
		}

		if len(tlsConfig.Certificates) > 0 {
			httpsServer := &http.Server{
				Addr:      fmt.Sprintf(":%d", s.config.TLSPort),
				Handler:   mux,
				TLSConfig: tlsConfig,
			}

			go func() {
				log.Printf("MCP Discovery HTTPS server listening on port %d", s.config.TLSPort)
				if err := httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
					log.Printf("HTTPS server error: %v", err)
				}
			}()
		}
	}

	log.Printf("MCP Discovery service started successfully")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigChan:
		log.Println("Received shutdown signal")
	case <-s.shutdownCh:
		log.Println("Received internal shutdown signal")
	}

	return s.Stop()
}

// Stop stops the discovery service
func (s *Service) Stop() error {
	log.Println("Stopping MCP Discovery service")
	close(s.shutdownCh)

	// Cancel all active scans
	s.mu.Lock()
	for _, job := range s.activeScans {
		if job.cancel != nil {
			job.cancel()
		}
	}
	s.activeScans = nil
	s.mu.Unlock()

	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return s.server.Shutdown(ctx)
	}
	return nil
}

// generateSelfSignedCert generates a self-signed certificate for development
func (s *Service) generateSelfSignedCert() (*tls.Certificate, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"SUSE AI Universal Proxy"},
			CommonName:   "localhost",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost", "127.0.0.1"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, err
	}

	cert := &tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  privateKey,
	}

	return cert, nil
}

// handleHealth handles health check requests
func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"service":   "discovery",
		"timestamp": time.Now(),
	})
}

// handleStartScan handles scan initiation requests
func (s *Service) handleStartScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var scanConfig models.ScanConfig
	if err := json.NewDecoder(r.Body).Decode(&scanConfig); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Set defaults
	if scanConfig.Timeout == "" {
		scanConfig.Timeout = s.config.DefaultTimeout.String()
	}
	if scanConfig.MaxConcurrent == 0 {
		scanConfig.MaxConcurrent = s.config.MaxConcurrency
	}
	if scanConfig.ExcludeProxy == nil {
		excludeProxy := s.config.ExcludeProxy
		scanConfig.ExcludeProxy = &excludeProxy
	}

	// Generate scan ID
	scanID := fmt.Sprintf("scan-%d", time.Now().UnixNano())

	// Create context for the scan
	ctx, cancel := context.WithCancel(context.Background())

	// Create scan job
	job := &ScanJob{
		ID:        scanID,
		Config:    scanConfig,
		StartTime: time.Now(),
		Status:    "running",
		cancel:    cancel,
	}

	// Store active scan
	s.mu.Lock()
	s.activeScans[scanID] = job
	s.mu.Unlock()

	// Start scan asynchronously
	go s.performScan(ctx, job)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"scanId":    scanID,
		"status":    "started",
		"config":    scanConfig,
		"startTime": job.StartTime,
	})
}

// performScan executes the actual network scan
func (s *Service) performScan(ctx context.Context, job *ScanJob) {
	defer func() {
		s.mu.Lock()
		delete(s.activeScans, job.ID)
		s.mu.Unlock()
	}()

	// Create scanner with the scan config
	scanner := scanner.NewNetworkScanner(&job.Config)

	// Perform the scan
	results, scanErrors := scanner.Scan()

	// Check for scan errors
	if len(scanErrors) > 0 {
		job.Status = "failed"
		job.Error = fmt.Sprintf("Scan errors: %v", scanErrors)
		log.Printf("Scan %s failed with errors: %v", job.ID, scanErrors)
		return
	}

	// Store results
	job.Results = results
	job.Status = "completed"
	log.Printf("Scan %s completed: found %d servers", job.ID, len(job.Results))

	// Store scan results
	if err := s.store.SaveScan(job.ID, job.Config, job.Results); err != nil {
		log.Printf("Failed to save scan results for %s: %v", job.ID, err)
	}
}

// handleGetScanStatus handles requests for scan status
func (s *Service) handleGetScanStatus(w http.ResponseWriter, r *http.Request) {
	scanID := strings.TrimPrefix(r.URL.Path, "/api/v1/scan/")
	if scanID == "" {
		http.NotFound(w, r)
		return
	}

	s.mu.RLock()
	job, isActive := s.activeScans[scanID]
	s.mu.RUnlock()

	var response map[string]interface{}

	if isActive {
		// Active scan
		response = map[string]interface{}{
			"scanId":    job.ID,
			"status":    job.Status,
			"startTime": job.StartTime,
			"config":    job.Config,
		}
		if job.Error != "" {
			response["error"] = job.Error
		}
	} else {
		// Check stored results
		config, results, err := s.store.GetScan(scanID)
		if err != nil {
			http.Error(w, "Scan not found", http.StatusNotFound)
			return
		}

		response = map[string]interface{}{
			"scanId":      scanID,
			"status":      "completed",
			"serverCount": len(results),
			"results":     results,
			"config":      config,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleListServers handles requests to list all discovered servers
func (s *Service) handleListServers(w http.ResponseWriter, r *http.Request) {
	// Get all stored scans and aggregate their results
	scans := s.store.ListScans()
	allServers := make([]models.DiscoveredServer, 0)

	for _, scanID := range scans {
		_, results, err := s.store.GetScan(scanID)
		if err != nil {
			continue
		}
		allServers = append(allServers, results...)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"servers":    allServers,
		"totalCount": len(allServers),
		"scanCount":  len(scans),
	})
}
