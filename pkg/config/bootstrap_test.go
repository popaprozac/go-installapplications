package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	return p
}

func TestValidateUnknownTypeFails(t *testing.T) {
	// unknown type should fail validation
	bootstrap := &Bootstrap{
		Userland: []Item{{Name: "Bad", File: "/tmp/foo", Type: "foo"}},
	}
	if err := ValidateBootstrap(bootstrap); err == nil {
		t.Fatalf("expected validation error for unknown type, got nil")
	}
}

func TestPreflightOnlyRootscript(t *testing.T) {
	// preflight must be exactly rootscript when present
	bootstrap := &Bootstrap{Preflight: []Item{{Name: "pkg", File: "/tmp/x.pkg", Type: "package"}}}
	if err := ValidateBootstrap(bootstrap); err == nil {
		t.Fatalf("expected error for non-rootscript in preflight, got nil")
	}
	bootstrap = &Bootstrap{Preflight: []Item{{Name: "ok", File: "/tmp/a.sh", Type: "rootscript"}}}
	if err := ValidateBootstrap(bootstrap); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadBootstrapWithSkipValidation(t *testing.T) {
	tdir := t.TempDir()
	// Create a bootstrap with an invalid type
	content := `{"userland":[{"name":"Bad","file":"/tmp/foo","type":"foo"}]}`
	path := writeTemp(t, tdir, "bootstrap.json", content)

	// With validation: should error
	if _, err := LoadBootstrap(path); err == nil {
		t.Fatalf("expected error with validation enabled")
	}
	// Without validation: should load
	if _, err := LoadBootstrapWithOptions(path, false); err != nil {
		t.Fatalf("unexpected error with validation disabled: %v", err)
	}
}
