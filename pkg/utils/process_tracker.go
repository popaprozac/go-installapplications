package utils

import (
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// BackgroundProcess represents a tracked background process
type BackgroundProcess struct {
	Cmd     *exec.Cmd
	Name    string
	Started time.Time
}

// ProcessTracker manages background processes started with donotwait
type ProcessTracker struct {
	processes []BackgroundProcess
	mutex     sync.Mutex
	logger    *Logger
}

// NewProcessTracker creates a new process tracker
func NewProcessTracker(logger *Logger) *ProcessTracker {
	return &ProcessTracker{
		processes: make([]BackgroundProcess, 0),
		logger:    logger,
	}
}

// StartBackgroundProcess starts a process in the background and tracks it
func (pt *ProcessTracker) StartBackgroundProcess(cmd *exec.Cmd, name string) error {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	// Start the process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start background process %s: %w", name, err)
	}

	// Track it
	bgProcess := BackgroundProcess{
		Cmd:     cmd,
		Name:    name,
		Started: time.Now(),
	}

	pt.processes = append(pt.processes, bgProcess)
	pt.logger.Info("Started background process: %s (PID: %d)", name, cmd.Process.Pid)
	pt.logger.Debug("Now tracking %d background processes", len(pt.processes))

	return nil
}

// WaitForCompletion waits for all tracked background processes to complete
func (pt *ProcessTracker) WaitForCompletion(timeout time.Duration) []error {
	pt.mutex.Lock()
	if len(pt.processes) == 0 {
		pt.mutex.Unlock()
		pt.logger.Debug("No background processes to wait for")
		return nil
	}

	// Get current processes and prepare to clear them after waiting
	processes := make([]BackgroundProcess, len(pt.processes))
	copy(processes, pt.processes)
	pt.mutex.Unlock()

	pt.logger.Info("Waiting for %d background processes to complete (timeout: %v)", len(processes), timeout)

	// Create channels for completion tracking
	done := make(chan int, len(processes))
	var errors []error
	var errorMutex sync.Mutex

	// Wait for each process in a separate goroutine
	for i, bgProcess := range processes {
		go func(index int, bp BackgroundProcess) {
			pt.logger.Verbose("Waiting for background process: %s", bp.Name)

			err := bp.Cmd.Wait()
			runtime := time.Since(bp.Started)

			if err != nil {
				pt.logger.Error("Background process %s failed after %v: %v", bp.Name, runtime, err)
				errorMutex.Lock()
				errors = append(errors, fmt.Errorf("background process %s failed: %w", bp.Name, err))
				errorMutex.Unlock()
			} else {
				pt.logger.Info("✅ Background process completed: %s (runtime: %v)", bp.Name, runtime)
			}

			done <- index
		}(i, bgProcess)
	}

	// Wait for completion or timeout
	completed := 0
	timeoutChan := time.After(timeout)

	for completed < len(processes) {
		select {
		case <-done:
			completed++
			pt.logger.Debug("Background process completed (%d/%d)", completed, len(processes))

		case <-timeoutChan:
			pt.logger.Error("Timeout waiting for background processes (%d/%d completed)", completed, len(processes))

			// Kill remaining processes
			for _, bgProcess := range processes {
				if bgProcess.Cmd.ProcessState == nil || !bgProcess.Cmd.ProcessState.Exited() {
					pt.logger.Error("Killing timed-out background process: %s", bgProcess.Name)
					bgProcess.Cmd.Process.Kill()
				}
			}

			errorMutex.Lock()
			errors = append(errors, fmt.Errorf("timeout waiting for %d background processes", len(processes)-completed))
			errorMutex.Unlock()

			// Clear processes even on timeout to prevent future issues
			pt.mutex.Lock()
			pt.processes = pt.processes[:0]
			pt.logger.Debug("Cleared timed-out background processes from tracker")
			pt.mutex.Unlock()

			return errors
		}
	}

	pt.logger.Info("All %d background processes completed", len(processes))

	// Clear completed processes from tracker to prevent "Wait was already called" errors
	pt.mutex.Lock()
	pt.processes = pt.processes[:0] // Clear the slice
	pt.logger.Debug("Cleared completed background processes from tracker")
	pt.mutex.Unlock()

	return errors
}

// GetActiveCount returns the number of currently tracked processes
func (pt *ProcessTracker) GetActiveCount() int {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()
	return len(pt.processes)
}
