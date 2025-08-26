package download

import (
	"fmt"
	"sync"

	"github.com/go-installapplications/pkg/config"
)

// DownloadResult represents the result of a download operation
type DownloadResult struct {
	Item  config.Item
	Error error
}

// DownloadMultipleWithCleanup downloads items in parallel with cleanup on failure
func (c *Client) DownloadMultipleWithCleanup(items []config.Item, maxConcurrency int, cleanupOnFailure bool) []DownloadResult {
	if maxConcurrency <= 0 {
		maxConcurrency = len(items)
	}

	var wg sync.WaitGroup
	results := make([]DownloadResult, len(items))
	semaphore := make(chan struct{}, maxConcurrency)

	// Create cleanup tracker
	cleanup := NewCleanupTracker()

	for i, item := range items {
		wg.Add(1)

		go func(index int, item config.Item) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			c.logger.Debug("Starting download: %s", item.Name)

			if item.URL != "" {
				// Track file for potential cleanup
				cleanup.TrackFile(item.File)

				// Use item-specific retry settings
				c.logger.Verbose("Item retry settings - Retries: %d, RetryWait: %ds", item.Retries, item.RetryWait)
				err := c.DownloadFileWithRetries(item.URL, item.File, item.Hash, item.Retries, item.RetryWait)
				if err != nil {
					results[index] = DownloadResult{Item: item, Error: err}
				} else {
					cleanup.MarkSuccess(item.File)
					results[index] = DownloadResult{Item: item, Error: nil}
				}
			} else {
				results[index] = DownloadResult{Item: item, Error: nil}
			}
		}(i, item)
	}

	wg.Wait()

	// Check if any downloads failed
	var failedCount int
	for _, result := range results {
		if result.Error != nil {
			failedCount++
		}
	}

	// Cleanup failed files if requested and there were failures
	if cleanupOnFailure && failedCount > 0 {
		fmt.Printf("Cleaning up %d failed downloads...\n", failedCount)
		if err := cleanup.Cleanup(); err != nil {
			fmt.Printf("Warning: cleanup failed: %v\n", err)
		}
	}

	return results
}
