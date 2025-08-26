package retry

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const (
	RetryCounterFile = "/var/tmp/go-installapplications/.retry-state"
	MaxRetries       = 3
)

// RetryState tracks daemon retry attempts
type RetryState struct {
	Count    int       `json:"count"`
	FirstTry time.Time `json:"first_try"`
	LastTry  time.Time `json:"last_try"`
	Reason   string    `json:"reason,omitempty"`
}

// GetRetryCount returns current retry count
func GetRetryCount() int {
	state, err := readRetryState()
	if err != nil {
		return 0 // First attempt
	}
	return state.Count
}

// IncrementRetryCount increments and saves retry count
func IncrementRetryCount(reason string) error {
	state, err := readRetryState()
	if err != nil {
		// First attempt
		state = &RetryState{
			Count:    0,
			FirstTry: time.Now(),
		}
	}

	state.Count++
	state.LastTry = time.Now()
	state.Reason = reason

	return saveRetryState(state)
}

// ClearRetryCount removes retry state (successful completion)
func ClearRetryCount() error {
	return os.Remove(RetryCounterFile)
}

// ShouldRetry checks if we should attempt retry
func ShouldRetry() (bool, error) {
	count := GetRetryCount()
	if count >= MaxRetries {
		return false, fmt.Errorf("maximum retry attempts (%d) exceeded", MaxRetries)
	}
	return true, nil
}

// GetRetryInfo returns human-readable retry information
func GetRetryInfo() string {
	state, err := readRetryState()
	if err != nil {
		return "First attempt"
	}

	return fmt.Sprintf("Retry %d/%d (first attempt: %s, last: %s)",
		state.Count, MaxRetries,
		state.FirstTry.Format("15:04:05"),
		state.LastTry.Format("15:04:05"))
}

// readRetryState reads retry state from file
func readRetryState() (*RetryState, error) {
	data, err := os.ReadFile(RetryCounterFile)
	if err != nil {
		return nil, err
	}

	var state RetryState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

// saveRetryState saves retry state to file
func saveRetryState(state *RetryState) error {
	// Ensure directory exists
	if err := os.MkdirAll("/var/tmp/go-installapplications", 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(RetryCounterFile, data, 0644)
}
