package download

import "github.com/go-installapplications/pkg/config"

// Downloader defines what a downloader should be able to do
type Downloader interface {
	DownloadFile(url, filepath, expectedHash string) error
	DownloadFileWithRetries(url, filepath, expectedHash string, retries int, retryWait int) error
	VerifyFileHash(filepath, expectedHash string) error
	DownloadMultipleWithCleanup(items []config.Item, maxConcurrency int, cleanupOnFailure bool) []DownloadResult
}
