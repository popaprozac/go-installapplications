package download

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-installapplications/pkg/utils"
)

func TestSetFollowRedirects(t *testing.T) {
	// server that redirects to /final
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	defer final.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer redirect.Close()

	tmp := t.TempDir()
	dest := filepath.Join(tmp, "out.txt")
	logger := utils.NewLogger(false, false)
	c := NewClient(logger)

	// follow on
	c.SetFollowRedirects(true)
	if err := c.DownloadFile(redirect.URL, dest, ""); err != nil {
		t.Fatalf("follow=true: unexpected error: %v", err)
	}

	// follow off -> expect 302 not followed, thus non-200 error
	c.SetFollowRedirects(false)
	if err := c.DownloadFile(redirect.URL, dest, ""); err == nil {
		t.Fatalf("follow=false: expected error but got nil")
	}
}

func TestVerifyFileHash(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "f.bin")
	content := []byte("hello")
	if err := os.WriteFile(p, content, 0644); err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(content)
	expected := fmt.Sprintf("%x", sum[:])

	c := NewClient(utils.NewLogger(false, false))
	if err := c.VerifyFileHash(p, expected); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := c.VerifyFileHash(p, "deadbeef"); err == nil {
		t.Fatalf("expected mismatch error")
	}
}
