package download

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-installapplications/pkg/utils"
)

// HashCheckPolicy controls the verifier's treatment of missing/mismatching hashes.
type HashCheckPolicy int

const (
	// HashCheckWarning accepts missing hashes (with a log warning) and fails on
	// mismatch. This is the default and matches pre-policy behavior.
	HashCheckWarning HashCheckPolicy = iota
	// HashCheckStrict requires a hash on every download with a URL and fails on
	// mismatch. Closest to original Python InstallApplications behavior.
	HashCheckStrict
	// HashCheckIgnore silently accepts missing hashes and logs (but does not
	// fail on) mismatches.
	HashCheckIgnore
)

// ParseHashCheckPolicy maps user-supplied strings (case-insensitive) to a
// HashCheckPolicy. Unknown values default to Warning so a typo in a config
// can't accidentally weaken or harden security without a clear signal in logs.
func ParseHashCheckPolicy(s string) HashCheckPolicy {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "strict":
		return HashCheckStrict
	case "ignore":
		return HashCheckIgnore
	default:
		return HashCheckWarning
	}
}

// String returns the canonical name for a HashCheckPolicy.
func (p HashCheckPolicy) String() string {
	switch p {
	case HashCheckStrict:
		return "Strict"
	case HashCheckIgnore:
		return "Ignore"
	default:
		return "Warning"
	}
}

// Client handles HTTP downloads
type Client struct {
	httpClient       *http.Client
	logger           *utils.Logger
	authUser         string
	authPassword     string
	customHeaders    map[string]string
	defaultRetries   int
	defaultRetryWait int // seconds
	followRedirects  bool
	hashPolicy       HashCheckPolicy
}

// NewClient creates a new download client
func NewClient(logger *utils.Logger) *Client {
	client := &Client{
		httpClient:       &http.Client{CheckRedirect: nil},
		logger:           logger,
		customHeaders:    make(map[string]string),
		defaultRetries:   3,
		defaultRetryWait: 5,
		followRedirects:  false, // Default to false to match config
	}
	// Set the HTTP client to not follow redirects by default
	client.SetFollowRedirects(false)
	return client
}

// NewClientWithAuth creates a download client with HTTP authentication
func NewClientWithAuth(logger *utils.Logger, authUser, authPassword string, headers map[string]string) *Client {
	client := &Client{
		httpClient:       &http.Client{CheckRedirect: nil},
		logger:           logger,
		authUser:         authUser,
		authPassword:     authPassword,
		customHeaders:    make(map[string]string),
		defaultRetries:   3,
		defaultRetryWait: 5,
		followRedirects:  false, // Default to false to match config
	}

	// Set the HTTP client to not follow redirects by default
	client.SetFollowRedirects(false)

	// Copy custom headers
	for k, v := range headers {
		client.customHeaders[k] = v
	}

	return client
}

// SetFollowRedirects toggles HTTP redirect following
func (c *Client) SetFollowRedirects(follow bool) {
	c.followRedirects = follow
	if follow {
		c.httpClient.CheckRedirect = nil
	} else {
		c.httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // do not follow
		}
	}
}

// SetRetryDefaults sets the default retry count and delay (seconds) used when an item doesn't specify them
func (c *Client) SetRetryDefaults(retries, retryWaitSeconds int) {
	if retries > 0 {
		c.defaultRetries = retries
	}
	if retryWaitSeconds > 0 {
		c.defaultRetryWait = retryWaitSeconds
	}
}

// SetHashCheckPolicy configures how the verifier handles missing/mismatching hashes.
func (c *Client) SetHashCheckPolicy(p HashCheckPolicy) {
	c.hashPolicy = p
}

// DownloadFileWithRetries downloads a file with item-specific retry settings
func (c *Client) DownloadFileWithRetries(url, filepath, expectedHash string, retries int, retryWait int) error {
	c.logger.Debug("Downloading %s to %s", url, filepath)

	// Use client defaults if not specified
	if retries == 0 {
		retries = c.defaultRetries
	}
	if retryWait == 0 {
		retryWait = c.defaultRetryWait
	}

	c.logger.Debug("Using retry settings: %d retries, %d second delay", retries, retryWait)

	// Create the retry operation as a closure
	downloadOperation := func() error {
		return c.downloadOnce(url, filepath)
	}

	// Use item-specific retry logic
	retryDuration := time.Duration(retryWait) * time.Second
	attempts, err := utils.Retry(downloadOperation, retries, retryDuration, fmt.Sprintf("download %s", url), c.logger)
	if err != nil {
		return err
	}

	c.logger.Debug("Download completed in %d attempts", attempts)

	// Verify hash if provided
	if err := c.VerifyFileHash(filepath, expectedHash); err != nil {
		return err
	}

	return nil
}

// DownloadFile downloads a single file using the client's configured retry defaults
// (set via SetRetryDefaults). Passing 0 for retries/retryWait lets DownloadFileWithRetries
// pick up the configured defaults instead of hard-coded values.
func (c *Client) DownloadFile(url, filepath, expectedHash string) error {
	return c.DownloadFileWithRetries(url, filepath, expectedHash, 0, 0)
}

// VerifyFileHash checks if a file matches the expected SHA256 hash. The
// behavior for missing/mismatching hashes depends on the client's configured
// HashCheckPolicy (default: Warning).
func (c *Client) VerifyFileHash(filepath, expectedHash string) error {
	if expectedHash == "" {
		switch c.hashPolicy {
		case HashCheckStrict:
			return fmt.Errorf("hash is required for %s under HashCheckPolicy=Strict", filepath)
		case HashCheckIgnore:
			c.logger.Debug("No hash provided for %s; HashCheckPolicy=Ignore — accepting", filepath)
			return nil
		default: // Warning
			c.logger.Info("⚠️  No hash provided for %s; HashCheckPolicy=Warning — accepting (use Strict to require)", filepath)
			return nil
		}
	}

	c.logger.Debug("Verifying hash for %s", filepath)
	c.logger.Verbose("Expected hash: %s", expectedHash)

	// Open the file
	file, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("failed to open file for hash verification: %w", err)
	}
	defer file.Close()

	// Create SHA256 hasher
	hasher := sha256.New()

	// Copy file contents to hasher
	_, err = io.Copy(hasher, file)
	if err != nil {
		return fmt.Errorf("failed to read file for hashing: %w", err)
	}

	// Get the hash as a hex string
	actualHash := fmt.Sprintf("%x", hasher.Sum(nil))
	c.logger.Verbose("Calculated hash: %s", actualHash)

	// Compare hashes
	if actualHash != expectedHash {
		if c.hashPolicy == HashCheckIgnore {
			c.logger.Info("⚠️  Hash mismatch for %s (expected %s, got %s); HashCheckPolicy=Ignore — accepting", filepath, expectedHash, actualHash)
			return nil
		}
		return fmt.Errorf("hash mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	c.logger.Debug("Hash verification passed for %s", filepath)
	return nil
}

// downloadOnce performs a single download attempt
func (c *Client) downloadOnce(url, filepath string) error {
	c.logger.Debug("Making HTTP request to %s", url)

	// Ensure the directory exists
	if err := utils.EnsureDirForFile(filepath); err != nil {
		return err
	}

	// Create HTTP request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request for %s: %w", url, err)
	}

	// Add HTTP Basic Authentication if configured
	if c.authUser != "" && c.authPassword != "" {
		req.SetBasicAuth(c.authUser, c.authPassword)
		c.logger.Debug("Added HTTP Basic Auth for user: %s", c.authUser)
	}

	// Add custom headers (sanitize secrets in logs)
	for key, value := range c.customHeaders {
		req.Header.Set(key, value)
		if key == "Authorization" || key == "Proxy-Authorization" {
			c.logger.Verbose("Added custom header: %s", key)
		} else {
			c.logger.Verbose("Added custom header: %s", key)
		}
	}

	req.Header.Set("User-Agent", "go-installapplications/1.0")

	// Log request headers in verbose mode (mask secret values)
	if c.logger != nil {
		safe := make(http.Header)
		for k, vals := range req.Header {
			if k == "Authorization" || k == "Proxy-Authorization" {
				safe[k] = []string{"***redacted***"}
			} else {
				safe[k] = vals
			}
		}
		c.logger.Verbose("HTTP request headers: %v", safe)
	}

	// Make HTTP request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", url, err)
	}
	defer resp.Body.Close()

	c.logger.Debug("HTTP response status: %d", resp.StatusCode)
	c.logger.Verbose("HTTP response headers: %v", resp.Header)

	// Check if request was successful
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	// Create the output file
	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", filepath, err)
	}
	defer file.Close()

	// Copy data from response to file
	bytesWritten, err := io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	c.logger.Debug("Downloaded %d bytes to %s", bytesWritten, filepath)
	return nil
}
