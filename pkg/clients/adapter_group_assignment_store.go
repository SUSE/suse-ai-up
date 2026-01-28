package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"suse-ai-up/pkg/models"
)

// FileAdapterGroupAssignmentStore implements AdapterGroupAssignmentStore using file-based storage
type FileAdapterGroupAssignmentStore struct {
	filePath    string
	assignments map[string]models.AdapterGroupAssignment // key: adapterID:groupID
	mu          sync.RWMutex
}

// NewFileAdapterGroupAssignmentStore creates a new file-based adapter group assignment store
func NewFileAdapterGroupAssignmentStore(filePath string) *FileAdapterGroupAssignmentStore {
	store := &FileAdapterGroupAssignmentStore{
		filePath:    filePath,
		assignments: make(map[string]models.AdapterGroupAssignment),
	}

	// Load existing assignments from file
	if err := store.loadFromFile(); err != nil {
		// Log error but continue with empty store
		fmt.Printf("Warning: Failed to load adapter group assignments from file: %v\n", err)
	}

	return store
}

// generateAssignmentID generates a unique ID for an assignment
func generateAssignmentID(adapterID, groupID string) string {
	return fmt.Sprintf("%s:%s", adapterID, groupID)
}

// CreateAssignment stores a new adapter group assignment
func (s *FileAdapterGroupAssignmentStore) CreateAssignment(ctx context.Context, assignment models.AdapterGroupAssignment) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := generateAssignmentID(assignment.AdapterID, assignment.GroupID)
	if _, exists := s.assignments[id]; exists {
		return fmt.Errorf("assignment for adapter %s and group %s already exists", assignment.AdapterID, assignment.GroupID)
	}

	s.assignments[id] = assignment
	return s.saveToFile()
}

// GetAssignment retrieves an adapter group assignment by adapterID and groupID
func (s *FileAdapterGroupAssignmentStore) GetAssignment(ctx context.Context, adapterID, groupID string) (*models.AdapterGroupAssignment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id := generateAssignmentID(adapterID, groupID)
	assignment, exists := s.assignments[id]
	if !exists {
		return nil, fmt.Errorf("assignment for adapter %s and group %s not found", adapterID, groupID)
	}

	// Return a copy to prevent external modifications
	assignmentCopy := assignment
	return &assignmentCopy, nil
}

// UpdateAssignment modifies an existing adapter group assignment
func (s *FileAdapterGroupAssignmentStore) UpdateAssignment(ctx context.Context, assignment models.AdapterGroupAssignment) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := generateAssignmentID(assignment.AdapterID, assignment.GroupID)
	if _, exists := s.assignments[id]; !exists {
		return fmt.Errorf("assignment for adapter %s and group %s not found", assignment.AdapterID, assignment.GroupID)
	}

	s.assignments[id] = assignment
	return s.saveToFile()
}

// DeleteAssignment removes an adapter group assignment
func (s *FileAdapterGroupAssignmentStore) DeleteAssignment(ctx context.Context, adapterID, groupID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := generateAssignmentID(adapterID, groupID)
	if _, exists := s.assignments[id]; !exists {
		return fmt.Errorf("assignment for adapter %s and group %s not found", adapterID, groupID)
	}

	delete(s.assignments, id)
	return s.saveToFile()
}

// ListAssignments retrieves all adapter group assignments
func (s *FileAdapterGroupAssignmentStore) ListAssignments(ctx context.Context) ([]models.AdapterGroupAssignment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	allAssignments := make([]models.AdapterGroupAssignment, 0, len(s.assignments))
	for _, assignment := range s.assignments {
		allAssignments = append(allAssignments, assignment)
	}

	return allAssignments, nil
}

// ListAssignmentsForAdapter retrieves all group assignments for a specific adapter
func (s *FileAdapterGroupAssignmentStore) ListAssignmentsForAdapter(ctx context.Context, adapterID string) ([]models.AdapterGroupAssignment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	adapterAssignments := make([]models.AdapterGroupAssignment, 0)
	for _, assignment := range s.assignments {
		if assignment.AdapterID == adapterID {
			adapterAssignments = append(adapterAssignments, assignment)
		}
	}

	return adapterAssignments, nil
}

// ListAssignmentsForGroup retrieves all adapter assignments for a specific group
func (s *FileAdapterGroupAssignmentStore) ListAssignmentsForGroup(ctx context.Context, groupID string) ([]models.AdapterGroupAssignment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	groupAssignments := make([]models.AdapterGroupAssignment, 0)
	for _, assignment := range s.assignments {
		if assignment.GroupID == groupID {
			groupAssignments = append(groupAssignments, assignment)
		}
	}

	return groupAssignments, nil
}

// HasAccess checks if a group has access to an adapter
func (s *FileAdapterGroupAssignmentStore) HasAccess(ctx context.Context, adapterID, groupID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id := generateAssignmentID(adapterID, groupID)
	assignment, exists := s.assignments[id]
	if !exists {
		return false, nil
	}

	// For now, any assignment means access with "read" permission
	return assignment.Permission == "read", nil
}

// loadFromFile loads assignments from the JSON file
func (s *FileAdapterGroupAssignmentStore) loadFromFile() error {
	if _, err := os.Stat(s.filePath); os.IsNotExist(err) {
		// File doesn't exist, start with empty store
		return nil
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return fmt.Errorf("failed to read adapter group assignment file: %w", err)
	}

	if len(data) == 0 {
		// Empty file, start with empty store
		return nil
	}

	var assignments []models.AdapterGroupAssignment
	if err := json.Unmarshal(data, &assignments); err != nil {
		return fmt.Errorf("failed to parse adapter group assignment file: %w", err)
	}

	// Convert to map
	for _, assignment := range assignments {
		id := generateAssignmentID(assignment.AdapterID, assignment.GroupID)
		s.assignments[id] = assignment
	}

	return nil
}

// saveToFile saves assignments to the JSON file
func (s *FileAdapterGroupAssignmentStore) saveToFile() error {
	// Ensure directory exists
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create adapter group assignment directory: %w", err)
	}

	// Convert map to slice for JSON serialization
	assignments := make([]models.AdapterGroupAssignment, 0, len(s.assignments))
	for _, assignment := range s.assignments {
		assignments = append(assignments, assignment)
	}

	data, err := json.MarshalIndent(assignments, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal adapter group assignments: %w", err)
	}

	// Write to temporary file first, then rename for atomicity
	tempFile := s.filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary adapter group assignment file: %w", err)
	}

	if err := os.Rename(tempFile, s.filePath); err != nil {
		return fmt.Errorf("failed to rename adapter group assignment file: %w", err)
	}

	return nil
}

// InMemoryAdapterGroupAssignmentStore provides an in-memory implementation for testing
type InMemoryAdapterGroupAssignmentStore struct {
	assignments map[string]models.AdapterGroupAssignment
	mu          sync.RWMutex
}

// NewInMemoryAdapterGroupAssignmentStore creates a new in-memory adapter group assignment store
func NewInMemoryAdapterGroupAssignmentStore() *InMemoryAdapterGroupAssignmentStore {
	return &InMemoryAdapterGroupAssignmentStore{
		assignments: make(map[string]models.AdapterGroupAssignment),
	}
}

// CreateAssignment stores a new adapter group assignment in memory
func (s *InMemoryAdapterGroupAssignmentStore) CreateAssignment(ctx context.Context, assignment models.AdapterGroupAssignment) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := generateAssignmentID(assignment.AdapterID, assignment.GroupID)
	if _, exists := s.assignments[id]; exists {
		return fmt.Errorf("assignment for adapter %s and group %s already exists", assignment.AdapterID, assignment.GroupID)
	}

	s.assignments[id] = assignment
	return nil
}

// GetAssignment retrieves an adapter group assignment by adapterID and groupID from memory
func (s *InMemoryAdapterGroupAssignmentStore) GetAssignment(ctx context.Context, adapterID, groupID string) (*models.AdapterGroupAssignment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id := generateAssignmentID(adapterID, groupID)
	assignment, exists := s.assignments[id]
	if !exists {
		return nil, fmt.Errorf("assignment for adapter %s and group %s not found", adapterID, groupID)
	}

	assignmentCopy := assignment
	return &assignmentCopy, nil
}

// UpdateAssignment modifies an existing adapter group assignment in memory
func (s *InMemoryAdapterGroupAssignmentStore) UpdateAssignment(ctx context.Context, assignment models.AdapterGroupAssignment) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := generateAssignmentID(assignment.AdapterID, assignment.GroupID)
	if _, exists := s.assignments[id]; !exists {
		return fmt.Errorf("assignment for adapter %s and group %s not found", assignment.AdapterID, assignment.GroupID)
	}

	s.assignments[id] = assignment
	return nil
}

// DeleteAssignment removes an adapter group assignment from memory
func (s *InMemoryAdapterGroupAssignmentStore) DeleteAssignment(ctx context.Context, adapterID, groupID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := generateAssignmentID(adapterID, groupID)
	if _, exists := s.assignments[id]; !exists {
		return fmt.Errorf("assignment for adapter %s and group %s not found", adapterID, groupID)
	}

	delete(s.assignments, id)
	return nil
}

// ListAssignments retrieves all adapter group assignments from memory
func (s *InMemoryAdapterGroupAssignmentStore) ListAssignments(ctx context.Context) ([]models.AdapterGroupAssignment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	allAssignments := make([]models.AdapterGroupAssignment, 0, len(s.assignments))
	for _, assignment := range s.assignments {
		allAssignments = append(allAssignments, assignment)
	}

	return allAssignments, nil
}

// ListAssignmentsForAdapter retrieves all group assignments for a specific adapter from memory
func (s *InMemoryAdapterGroupAssignmentStore) ListAssignmentsForAdapter(ctx context.Context, adapterID string) ([]models.AdapterGroupAssignment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	adapterAssignments := make([]models.AdapterGroupAssignment, 0)
	for _, assignment := range s.assignments {
		if assignment.AdapterID == adapterID {
			adapterAssignments = append(adapterAssignments, assignment)
		}
	}

	return adapterAssignments, nil
}

// ListAssignmentsForGroup retrieves all adapter assignments for a specific group from memory
func (s *InMemoryAdapterGroupAssignmentStore) ListAssignmentsForGroup(ctx context.Context, groupID string) ([]models.AdapterGroupAssignment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	groupAssignments := make([]models.AdapterGroupAssignment, 0)
	for _, assignment := range s.assignments {
		if assignment.GroupID == groupID {
			groupAssignments = append(groupAssignments, assignment)
		}
	}

	return groupAssignments, nil
}

// HasAccess checks if a group has access to an adapter in memory
func (s *InMemoryAdapterGroupAssignmentStore) HasAccess(ctx context.Context, adapterID, groupID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id := generateAssignmentID(adapterID, groupID)
	assignment, exists := s.assignments[id]
	if !exists {
		return false, nil
	}

	// For now, any assignment means access with "read" permission
	return assignment.Permission == "read", nil
}
