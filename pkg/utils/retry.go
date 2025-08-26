package utils

import (
	"fmt"
	"time"
)

// RetryFunc represents a function that can be retried
type RetryFunc func() error

// Retry executes a function with retry logic
func Retry(operation RetryFunc, maxRetries int, delay time.Duration, description string, logger *Logger) (int, error) {
	var lastError error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			logger.Info("Retry attempt %d/%d for %s (waiting %v)\n", attempt, maxRetries, description, delay)
			time.Sleep(delay)
		}

		err := operation()
		if err == nil {
			if attempt > 0 {
				logger.Info("Succeeded on attempt %d for %s", attempt+1, description)
			} else {
				logger.Debug("Succeeded on first attempt for %s", description)
			}
			return attempt + 1, nil // Return actual attempts made
		}

		lastError = err
		if attempt < maxRetries {
			logger.Debug("Attempt %d failed for %s: %v", attempt+1, description, err)
		}
	}

	logger.Error("Failed after %d attempts for %s: %v", maxRetries+1, description, lastError)
	return maxRetries + 1, fmt.Errorf("failed after %d attempts: %w", maxRetries+1, lastError)
}
