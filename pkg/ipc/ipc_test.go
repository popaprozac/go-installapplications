package ipc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetAgentSocketPathForUID(t *testing.T) {
	got := GetAgentSocketPathForUID("501")
	want := filepath.Join(SocketDir, "agent-501.sock")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	// Empty UID degrades to "unknown" rather than producing a malformed path
	got = GetAgentSocketPathForUID("")
	if !strings.HasSuffix(got, "agent-unknown.sock") {
		t.Fatalf("empty uid path = %q", got)
	}
}

func TestEnsureSocketDir_CreatesDir(t *testing.T) {
	// Use a temp directory under TMPDIR so tests don't depend on /var/tmp permissions.
	tmp := t.TempDir()
	// Swap the package's socket dir for the test.
	original := SocketDirVar()
	SetSocketDir(filepath.Join(tmp, "go-ia-sockets"))
	t.Cleanup(func() { SetSocketDir(original) })

	if err := EnsureSocketDir(); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	info, err := os.Stat(SocketDirVar())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected directory")
	}
}
