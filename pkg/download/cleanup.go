package download

import (
	"fmt"
	"os"
	"sync"
)

// CleanupTracker keeps track of files that need cleanup
type CleanupTracker struct {
	mutex sync.Mutex
	files map[string]bool // filepath -> shouldDelete (true=delete on failure; false=preserve)
}

// NewCleanupTracker creates a new cleanup tracker
func NewCleanupTracker() *CleanupTracker {
	return &CleanupTracker{
		files: make(map[string]bool),
	}
}

// TrackFile adds a file to cleanup tracking
func (ct *CleanupTracker) TrackFile(filepath string) {
	ct.mutex.Lock()
	defer ct.mutex.Unlock()
	ct.files[filepath] = true
}

// MarkSuccess marks a file as successfully completed (don't delete)
func (ct *CleanupTracker) MarkSuccess(filepath string) {
	ct.mutex.Lock()
	defer ct.mutex.Unlock()
	ct.files[filepath] = false
}

// Cleanup removes all files marked for deletion
func (ct *CleanupTracker) Cleanup() error {
	ct.mutex.Lock()
	defer ct.mutex.Unlock()

	var errors []error
	for filepath, shouldDelete := range ct.files {
		if shouldDelete {
			fmt.Printf("Cleaning up failed file: %s\n", filepath)
			if err := os.Remove(filepath); err != nil && !os.IsNotExist(err) {
				errors = append(errors, fmt.Errorf("failed to cleanup %s: %w", filepath, err))
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("cleanup errors: %d files failed to delete", len(errors))
	}

	return nil
}

// CleanupAll removes all tracked files, regardless of success or failure
func (ct *CleanupTracker) CleanupAll() error {
	ct.mutex.Lock()
	defer ct.mutex.Unlock()

	var errors []error
	for filepath := range ct.files {
		if err := os.Remove(filepath); err != nil && !os.IsNotExist(err) {
			errors = append(errors, fmt.Errorf("failed to cleanup %s: %w", filepath, err))
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("cleanup errors: %d files failed to delete", len(errors))
	}
	return nil
}
