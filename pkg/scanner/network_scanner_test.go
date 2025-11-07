package scanner

import (
	"testing"
	"time"

	"suse-ai-up/pkg/models"
)

func TestNewNetworkScanner(t *testing.T) {
	config := &models.ScanConfig{
		ScanRanges:    []string{"127.0.0.1"},
		Ports:         []string{"8080"},
		MaxConcurrent: 5,
	}

	scanner := NewNetworkScanner(config)
	if scanner == nil {
		t.Fatal("NewNetworkScanner returned nil")
	}

	if scanner.config != config {
		t.Error("Scanner config not set correctly")
	}
}

func TestExpandPorts(t *testing.T) {
	scanner := NewNetworkScanner(&models.ScanConfig{})

	// Test single ports
	ports := []string{"80", "443", "8080"}
	result, err := scanner.expandPorts(ports)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("Expected 3 ports, got %d", len(result))
	}

	expected := []int{80, 443, 8080}
	for i, port := range expected {
		if result[i] != port {
			t.Errorf("Expected port %d at index %d, got %d", port, i, result[i])
		}
	}
}

func TestExpandIPRange(t *testing.T) {
	scanner := NewNetworkScanner(&models.ScanConfig{})

	// Test single IP
	result, err := scanner.expandIPRange("192.168.1.1")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(result) != 1 || result[0] != "192.168.1.1" {
		t.Errorf("Expected single IP, got %v", result)
	}

	// Test invalid IP range
	_, err = scanner.expandIPRange("invalid")
	if err == nil {
		t.Error("Expected error for invalid IP range")
	}
}

func TestShouldExcludeAddress(t *testing.T) {
	config := &models.ScanConfig{
		ExcludeAddresses: []string{"127.0.0.1", "localhost"},
	}
	scanner := NewNetworkScanner(config)

	if !scanner.shouldExcludeAddress("127.0.0.1") {
		t.Error("Expected 127.0.0.1 to be excluded")
	}

	if !scanner.shouldExcludeAddress("localhost") {
		t.Error("Expected localhost to be excluded")
	}

	if scanner.shouldExcludeAddress("192.168.1.1") {
		t.Error("Expected 192.168.1.1 to not be excluded")
	}
}

func TestScanTimeout(t *testing.T) {
	config := &models.ScanConfig{
		ScanRanges:    []string{"10.255.255.1"}, // Unreachable IP
		Ports:         []string{"12345"},        // Unlikely to be open
		MaxConcurrent: 1,
	}

	scanner := NewNetworkScanner(config)

	// Start scan with timeout
	done := make(chan bool)
	go func() {
		scanner.Scan()
		done <- true
	}()

	select {
	case <-done:
		// Scan completed - this is expected since HTTP requests timeout
	case <-time.After(15 * time.Second):
		t.Error("Scan took too long, should have completed when HTTP requests timed out")
		scanner.Stop()
	}
}
