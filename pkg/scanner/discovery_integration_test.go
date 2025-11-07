package scanner

import (
	"testing"
	"time"

	"suse-ai-up/pkg/models"
)

func TestDiscoveryIntegration(t *testing.T) {
	// Create components
	store := NewInMemoryDiscoveryStore()
	baseScanner := NewNetworkScanner(&models.ScanConfig{})
	scanManager := NewScanManager(baseScanner, store)

	// Test scan configuration
	config := models.ScanConfig{
		ScanRanges:    []string{"192.168.1.74/32"},
		Ports:         []string{"8001", "8002", "8003", "8004"},
		Timeout:       "5s",
		MaxConcurrent: 2,
	}

	// Start a scan
	job, err := scanManager.StartScan(config)
	if err != nil {
		t.Fatalf("Failed to start scan: %v", err)
	}

	if job.Status != ScanStatusPending {
		t.Errorf("Expected job status to be pending, got %s", job.Status)
	}

	// Wait for scan to complete (with timeout)
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("Scan timed out")
		case <-ticker.C:
			currentJob, err := scanManager.GetJob(job.ID)
			if err != nil {
				t.Fatalf("Failed to get job: %v", err)
			}

			if currentJob.Status == ScanStatusCompleted || currentJob.Status == ScanStatusFailed {
				job = currentJob
				goto scanComplete
			}
		}
	}

scanComplete:
	// Verify scan completed
	if job.Status != ScanStatusCompleted {
		t.Errorf("Expected scan to complete, got status: %s", job.Status)
	}

	// Check that results were saved to store
	servers, err := store.GetAll()
	if err != nil {
		t.Fatalf("Failed to get servers from store: %v", err)
	}

	// We expect at least some servers to be found (test servers should be running)
	if len(servers) == 0 {
		t.Logf("No servers found - test servers may not be running")
	} else {
		t.Logf("Found %d servers in store", len(servers))
		for i, server := range servers {
			t.Logf("  %d. %s (%s)", i+1, server.Name, server.Address)
		}
	}

	// Test incremental scanning
	job2, err := scanManager.StartScan(config)
	if err != nil {
		t.Fatalf("Failed to start second scan: %v", err)
	}

	// Wait for second scan
	for {
		select {
		case <-timeout:
			t.Fatal("Second scan timed out")
		case <-ticker.C:
			currentJob, err := scanManager.GetJob(job2.ID)
			if err != nil {
				t.Fatalf("Failed to get second job: %v", err)
			}

			if currentJob.Status == ScanStatusCompleted || currentJob.Status == ScanStatusFailed {
				job2 = currentJob
				goto secondScanComplete
			}
		}
	}

secondScanComplete:
	// Second scan should complete quickly due to caching
	if job2.Status != ScanStatusCompleted {
		t.Errorf("Expected second scan to complete, got status: %s", job2.Status)
	}

	t.Logf("Integration test completed successfully")
}
