package mode

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-installapplications/pkg/config"
	"github.com/go-installapplications/pkg/utils"
)

func TestGetBootstrap_FollowRedirects(t *testing.T) {
	// final server returns minimal valid bootstrap
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"preflight":[{"file":"/tmp/a","name":"x","type":"rootscript"}]}`)
	}))
	defer final.Close()

	// redirector points to final
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer redirect.Close()

	cfg := config.NewConfig()
	cfg.JSONURL = redirect.URL
	cfg.InstallPath = t.TempDir()

	logger := utils.NewLogger(false, false)

	// No follow redirects => should fail
	cfg.FollowRedirects = false
	if _, err := getBootstrap(cfg, logger); err == nil {
		t.Fatalf("expected error when not following redirects")
	}

	// Follow redirects => should succeed
	cfg.FollowRedirects = true
	if _, err := getBootstrap(cfg, logger); err != nil {
		t.Fatalf("unexpected error with follow redirects: %v", err)
	}
}

func TestGetBootstrap_SkipValidation(t *testing.T) {
	// server returns invalid item type
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"userland":[{"file":"/tmp/a","name":"x","type":"foo"}]}`)
	}))
	defer srv.Close()

	cfg := config.NewConfig()
	cfg.JSONURL = srv.URL
	cfg.InstallPath = t.TempDir()
	cfg.FollowRedirects = true
	logger := utils.NewLogger(false, false)

	// validation on => expect error
	cfg.SkipValidation = false
	if _, err := getBootstrap(cfg, logger); err == nil {
		t.Fatalf("expected validation error")
	}
	// skip validation => should load
	cfg.SkipValidation = true
	if _, err := getBootstrap(cfg, logger); err != nil {
		t.Fatalf("unexpected error with skip-validation: %v", err)
	}
}
