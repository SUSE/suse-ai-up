package scanner

import (
	"fmt"
	"sync"
	"time"

	"suse-ai-up/pkg/models"
)

// ScanStatus represents the status of a scan job
type ScanStatus string

const (
	ScanStatusPending   ScanStatus = "pending"
	ScanStatusRunning   ScanStatus = "running"
	ScanStatusCompleted ScanStatus = "completed"
	ScanStatusFailed    ScanStatus = "failed"
	ScanStatusCancelled ScanStatus = "cancelled"
)

// ScanJob represents a scan job with its lifecycle
type ScanJob struct {
	ID        string                    `json:"id"`
	Status    ScanStatus                `json:"status"`
	Config    models.ScanConfig         `json:"config"`
	Results   []models.DiscoveredServer `json:"results,omitempty"`
	Errors    []error                   `json:"errors,omitempty"`
	StartTime time.Time                 `json:"startTime"`
	EndTime   *time.Time                `json:"endTime,omitempty"`
	Progress  float64                   `json:"progress"` // 0.0 to 1.0
	Message   string                    `json:"message,omitempty"`
}

// ScanManager manages scan jobs and coordinates scanning operations
type ScanManager struct {
	jobs       map[string]*ScanJob
	jobsMutex  sync.RWMutex
	scanner    *NetworkScanner
	store      DiscoveryStore
	nextJobID  int
	jobIDMutex sync.Mutex
}

// DiscoveryStore interface for persistent storage of discovered servers
type DiscoveryStore interface {
	Save(server *models.DiscoveredServer) error
	GetAll() ([]models.DiscoveredServer, error)
	GetByID(id string) (*models.DiscoveredServer, error)
	UpdateLastSeen(id string, lastSeen time.Time) error
	RemoveStale(threshold time.Duration) error
	Delete(id string) error
}

// NewScanManager creates a new scan manager
func NewScanManager(scanner *NetworkScanner, store DiscoveryStore) *ScanManager {
	return &ScanManager{
		jobs:      make(map[string]*ScanJob),
		scanner:   scanner,
		store:     store,
		nextJobID: 1,
	}
}

// StartScan initiates a new scan job
func (sm *ScanManager) StartScan(config models.ScanConfig) (*ScanJob, error) {
	sm.jobIDMutex.Lock()
	jobID := fmt.Sprintf("scan-%d", sm.nextJobID)
	sm.nextJobID++
	sm.jobIDMutex.Unlock()

	job := &ScanJob{
		ID:        jobID,
		Status:    ScanStatusPending,
		Config:    config,
		StartTime: time.Now(),
		Progress:  0.0,
		Message:   "Scan queued",
	}

	sm.jobsMutex.Lock()
	sm.jobs[jobID] = job
	sm.jobsMutex.Unlock()

	// Start the scan asynchronously
	go sm.runScan(job)

	return job, nil
}

// GetJob retrieves a scan job by ID
func (sm *ScanManager) GetJob(jobID string) (*ScanJob, error) {
	sm.jobsMutex.RLock()
	defer sm.jobsMutex.RUnlock()

	job, exists := sm.jobs[jobID]
	if !exists {
		return nil, fmt.Errorf("scan job not found: %s", jobID)
	}

	return job, nil
}

// ListJobs returns all scan jobs
func (sm *ScanManager) ListJobs() []*ScanJob {
	sm.jobsMutex.RLock()
	defer sm.jobsMutex.RUnlock()

	jobs := make([]*ScanJob, 0, len(sm.jobs))
	for _, job := range sm.jobs {
		jobs = append(jobs, job)
	}

	return jobs
}

// CancelJob cancels a running scan job
func (sm *ScanManager) CancelJob(jobID string) error {
	sm.jobsMutex.Lock()
	defer sm.jobsMutex.Unlock()

	job, exists := sm.jobs[jobID]
	if !exists {
		return fmt.Errorf("scan job not found: %s", jobID)
	}

	if job.Status == ScanStatusRunning {
		job.Status = ScanStatusCancelled
		job.Message = "Scan cancelled by user"
		now := time.Now()
		job.EndTime = &now
	}

	return nil
}

// runScan executes the scan job
func (sm *ScanManager) runScan(job *ScanJob) {
	// Update status to running
	sm.jobsMutex.Lock()
	job.Status = ScanStatusRunning
	job.Message = "Scan in progress"
	sm.jobsMutex.Unlock()

	// Create scanner with job config
	tempScanner := NewNetworkScanner(&job.Config)

	// Run the scan
	results, errors := tempScanner.Scan()

	// Update job with results
	sm.jobsMutex.Lock()
	now := time.Now()
	job.EndTime = &now
	job.Results = results
	job.Errors = errors
	job.Progress = 1.0

	if len(errors) > 0 && len(results) == 0 {
		job.Status = ScanStatusFailed
		job.Message = fmt.Sprintf("Scan failed with %d errors", len(errors))
	} else {
		job.Status = ScanStatusCompleted
		job.Message = fmt.Sprintf("Scan completed: found %d servers", len(results))
	}
	sm.jobsMutex.Unlock()

	// Save results to persistent store
	for _, server := range results {
		if err := sm.store.Save(&server); err != nil {
			// Log error but don't fail the job
			fmt.Printf("Failed to save server %s: %v\n", server.ID, err)
		}
	}
}

// CleanupOldJobs removes completed jobs older than the specified duration
func (sm *ScanManager) CleanupOldJobs(maxAge time.Duration) {
	sm.jobsMutex.Lock()
	defer sm.jobsMutex.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for id, job := range sm.jobs {
		if job.Status == ScanStatusCompleted || job.Status == ScanStatusFailed {
			if job.EndTime != nil && job.EndTime.Before(cutoff) {
				delete(sm.jobs, id)
			}
		}
	}
}
