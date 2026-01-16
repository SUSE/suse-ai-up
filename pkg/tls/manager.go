package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"time"
)

// Config holds TLS configuration
type Config struct {
	CertFile  string
	KeyFile   string
	AutoTLS   bool
	HTTPSPort int
}

// Manager handles TLS certificate management
type Manager struct {
	config *Config
	cert   tls.Certificate
}

// NewManager creates a new TLS manager
func NewManager(config *Config) (*Manager, error) {
	manager := &Manager{config: config}

	// Try to load existing certificates first
	if config.CertFile != "" && config.KeyFile != "" {
		if cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile); err == nil {
			manager.cert = cert
			log.Printf("Loaded TLS certificates from %s and %s", config.CertFile, config.KeyFile)
			return manager, nil
		}
		log.Printf("Failed to load certificates, will generate self-signed if AutoTLS is enabled")
	}

	// Generate self-signed certificate if AutoTLS is enabled
	if config.AutoTLS {
		cert, err := manager.generateSelfSignedCert()
		if err != nil {
			return nil, fmt.Errorf("failed to generate self-signed certificate: %w", err)
		}
		manager.cert = *cert
		log.Printf("Generated self-signed TLS certificate")
		return manager, nil
	}

	return nil, fmt.Errorf("no TLS certificates available and AutoTLS is disabled")
}

// GetCertificate returns the TLS certificate
func (m *Manager) GetCertificate() *tls.Certificate {
	return &m.cert
}

// GetTLSConfig returns a TLS config for HTTPS servers
func (m *Manager) GetTLSConfig() *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{m.cert},
		ServerName:   "localhost",
	}
}

// generateSelfSignedCert generates a self-signed certificate
func (m *Manager) generateSelfSignedCert() (*tls.Certificate, error) {
	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	// Create certificate template
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

	// Create certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, err
	}

	// Create TLS certificate
	cert := &tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  privateKey,
	}

	return cert, nil
}

// SaveCertificates saves the certificate and key to files
func (m *Manager) SaveCertificates(certFile, keyFile string) error {
	// Convert certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: m.cert.Certificate[0],
	})

	// Convert private key to PEM
	keyBytes, err := x509.MarshalPKCS8PrivateKey(m.cert.PrivateKey)
	if err != nil {
		return err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyBytes,
	})

	// Write certificate file
	if err := os.WriteFile(certFile, certPEM, 0644); err != nil {
		return err
	}

	// Write key file
	if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
		return err
	}

	log.Printf("Saved TLS certificates to %s and %s", certFile, keyFile)
	return nil
}
