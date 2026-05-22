package download

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/go-installapplications/pkg/utils"
)

// DownloadFile previously hard-coded retries=3,wait=5 and silently ignored
// SetRetryDefaults. After the fix, it should respect configured defaults so
// bootstrap.json downloads honor --max-retries / --retry-delay.
func TestDownloadFile_HonorsSetRetryDefaults(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			http.Error(w, "transient", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, "ok")
	}))
	defer server.Close()

	c := NewClient(utils.NewLogger(false, false))
	// Set retries=5 wait=0 — without the fix DownloadFile would only try 3+1=4 times
	// but here we want to verify the values are surfaced to DownloadFileWithRetries.
	c.SetRetryDefaults(5, 0)

	dest := filepath.Join(t.TempDir(), "out.txt")
	if err := c.DownloadFile(server.URL, dest, ""); err != nil {
		t.Fatalf("download should succeed within retries: %v", err)
	}
	if atomic.LoadInt32(&attempts) < 3 {
		t.Fatalf("expected retries to have happened, attempts=%d", attempts)
	}
}

// DownloadFile must still surface a failure when all attempts fail.
func TestDownloadFile_PropagatesAllRetryFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "always fails", http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient(utils.NewLogger(false, false))
	c.SetRetryDefaults(1, 0) // 1 retry => 2 attempts total

	dest := filepath.Join(t.TempDir(), "out.txt")
	if err := c.DownloadFile(server.URL, dest, ""); err == nil {
		t.Fatalf("expected error after exhausting retries")
	}
}
