package manager

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-installapplications/pkg/config"
	"github.com/go-installapplications/pkg/utils"
)

// countingInstaller records each call so we can prove skipped items are not
// executed. Counters are atomic so parallel_group tests don't race.
type countingInstaller struct {
	scripts  atomic.Int32
	packages atomic.Int32
	files    atomic.Int32
}

func (c *countingInstaller) callCount() int { return int(c.scripts.Load()) }

func (c *countingInstaller) InstallPackage(_, _ string) error                   { c.packages.Add(1); return nil }
func (c *countingInstaller) ExecuteScript(_, _ string, _ bool, _ bool) error    { c.scripts.Add(1); return nil }
func (c *countingInstaller) ExecuteScriptForPreflight(_, _ string, _ bool, _ bool) error {
	c.scripts.Add(1)
	return nil
}
func (c *countingInstaller) PlaceFile(_, _ string) error                          { c.files.Add(1); return nil }
func (c *countingInstaller) WaitForBackgroundProcesses(_ time.Duration) []error { return nil }
func (c *countingInstaller) GetBackgroundProcessCount() int                      { return 0 }

// TestManager_SkipIfFiltersBeforeExecution proves that items matching the
// current architecture's skip_if alias never reach the installer. This is the
// regression guard for the daemon's userland phase originally skipping the
// filter step.
func TestManager_SkipIfFiltersBeforeExecution(t *testing.T) {
	cfg := config.NewConfig()
	cfg.DownloadMaxConcurrency = 1
	logger := utils.NewLogger(false, false)

	// Pick the alias that matches the current architecture so the manager skips.
	var skipMine string
	if utils.IsAppleSilicon() {
		skipMine = "arm64"
	} else if utils.IsIntel() {
		skipMine = "intel"
	} else {
		t.Skip("unknown host architecture")
	}

	inst := &countingInstaller{}
	m := NewManager(&fakeDownloader{}, inst, cfg, logger)

	items := []config.Item{
		{Name: "run-me", File: "ok.sh", Type: "rootscript"},
		{Name: "skipped", File: "skipped.sh", Type: "rootscript", SkipIf: skipMine},
	}
	if err := m.ProcessItems(items, "userland"); err != nil {
		t.Fatalf("ProcessItems: %v", err)
	}
	if inst.callCount() != 1 {
		t.Fatalf("expected exactly one script (skip filter dropped one), got %d", inst.callCount())
	}
}

// TestManager_FailableTreatsFailureAsContinue is a behavioural test against the
// shared ShouldStopOnError helper as wired through the manager.
func TestManager_FailableTreatsFailureAsContinue(t *testing.T) {
	cfg := config.NewConfig()
	cfg.DownloadMaxConcurrency = 1
	logger := utils.NewLogger(false, false)

	inst := &fakeInstaller{}
	m := NewManager(&fakeDownloader{}, inst, cfg, logger)

	items := []config.Item{
		{Name: "first", File: "fail.sh", Type: "rootscript", FailPolicy: "failable"},
		{Name: "second", File: "ok.sh", Type: "rootscript", FailPolicy: "failable"},
	}
	if err := m.ProcessItems(items, "userland"); err != nil {
		t.Fatalf("failable should swallow errors: %v", err)
	}
	if inst.callCount() != 2 {
		t.Fatalf("expected both scripts to run, got %d", inst.callCount())
	}
}
