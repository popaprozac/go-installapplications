package installer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-installapplications/pkg/utils"
)

func writeTempFile(t *testing.T, name string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("hello"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	return p
}

func TestFilePlacer_RootFilePermissions(t *testing.T) {
	logger := utils.NewLogger(false, false)
	fp := NewFilePlacer(false, logger, false)

	path := writeTempFile(t, "root.txt", 0600)
	if err := fp.PlaceFile(path, "rootfile"); err != nil {
		t.Fatalf("place: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0644 {
		t.Fatalf("rootfile should be 0644, got %v", info.Mode().Perm())
	}
}

func TestFilePlacer_UserFilePermissions(t *testing.T) {
	logger := utils.NewLogger(false, false)
	fp := NewFilePlacer(false, logger, true)

	path := writeTempFile(t, "user.sh", 0600)
	if err := fp.PlaceFile(path, "userfile"); err != nil {
		t.Fatalf("place: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("userfile should be 0755, got %v", info.Mode().Perm())
	}
}

func TestFilePlacer_MissingFile(t *testing.T) {
	logger := utils.NewLogger(false, false)
	fp := NewFilePlacer(false, logger, false)
	err := fp.PlaceFile("/this/path/does/not/exist", "rootfile")
	if err == nil {
		t.Fatalf("expected error for missing file")
	}
}

func TestFilePlacer_UnknownType(t *testing.T) {
	logger := utils.NewLogger(false, false)
	fp := NewFilePlacer(false, logger, false)
	path := writeTempFile(t, "x", 0644)
	if err := fp.PlaceFile(path, "bogus"); err == nil {
		t.Fatalf("expected error for unknown file type")
	}
}

func TestFilePlacer_DryRunDoesNothing(t *testing.T) {
	logger := utils.NewLogger(false, false)
	fp := NewFilePlacer(true, logger, false)

	path := writeTempFile(t, "dr.txt", 0600)
	if err := fp.PlaceFile(path, "rootfile"); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// Dry run must not chmod.
	if info.Mode().Perm() != 0600 {
		t.Fatalf("dry-run should not change perms; got %v", info.Mode().Perm())
	}
}
