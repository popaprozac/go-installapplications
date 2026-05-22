package manager

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-installapplications/pkg/config"
	"github.com/go-installapplications/pkg/utils"
)

// recordingInstaller pretends to take time per call so we can prove that
// items in a parallel_group actually run concurrently.
type recordingInstaller struct {
	scriptCount int32
	maxInFlight int32
	inFlight    int32
	delay       time.Duration
}

func (r *recordingInstaller) trackEntry() {
	cur := atomic.AddInt32(&r.inFlight, 1)
	for {
		m := atomic.LoadInt32(&r.maxInFlight)
		if cur <= m || atomic.CompareAndSwapInt32(&r.maxInFlight, m, cur) {
			break
		}
	}
}
func (r *recordingInstaller) trackExit() { atomic.AddInt32(&r.inFlight, -1) }

func (r *recordingInstaller) InstallPackage(_, _ string) error { return nil }
func (r *recordingInstaller) ExecuteScript(_, _ string, _ bool, _ bool) error {
	r.trackEntry()
	defer r.trackExit()
	atomic.AddInt32(&r.scriptCount, 1)
	if r.delay > 0 {
		time.Sleep(r.delay)
	}
	return nil
}
func (r *recordingInstaller) ExecuteScriptForPreflight(_, _ string, _ bool, _ bool) error { return nil }
func (r *recordingInstaller) PlaceFile(_, _ string) error                                 { return nil }
func (r *recordingInstaller) WaitForBackgroundProcesses(_ time.Duration) []error          { return nil }
func (r *recordingInstaller) GetBackgroundProcessCount() int                              { return 0 }

func TestManager_ParallelGroupRunsConcurrently(t *testing.T) {
	cfg := config.NewConfig()
	cfg.DownloadMaxConcurrency = 4
	logger := utils.NewLogger(false, false)

	inst := &recordingInstaller{delay: 100 * time.Millisecond}
	m := NewManager(&fakeDownloader{}, inst, cfg, logger)

	items := []config.Item{
		{Name: "a1", File: "a.sh", Type: "rootscript", ParallelGroup: "alpha"},
		{Name: "a2", File: "a.sh", Type: "rootscript", ParallelGroup: "alpha"},
		{Name: "a3", File: "a.sh", Type: "rootscript", ParallelGroup: "alpha"},
	}
	start := time.Now()
	if err := m.ProcessItems(items, "userland"); err != nil {
		t.Fatalf("ProcessItems: %v", err)
	}
	elapsed := time.Since(start)

	if atomic.LoadInt32(&inst.scriptCount) != 3 {
		t.Fatalf("expected 3 script runs, got %d", inst.scriptCount)
	}
	if atomic.LoadInt32(&inst.maxInFlight) < 2 {
		t.Fatalf("expected concurrent execution (maxInFlight>=2), got %d", inst.maxInFlight)
	}
	// Sequential would be ~300ms; concurrent should finish well under that.
	if elapsed >= 250*time.Millisecond {
		t.Fatalf("parallel batch took too long (%v), expected concurrency", elapsed)
	}
}

func TestManager_ParallelGroupRespectsFailPolicyStrict(t *testing.T) {
	cfg := config.NewConfig()
	cfg.DownloadMaxConcurrency = 4
	logger := utils.NewLogger(false, false)

	// fakeInstaller from manager_test.go fails on "fail.sh"
	inst := &fakeInstaller{}
	m := NewManager(&fakeDownloader{}, inst, cfg, logger)

	items := []config.Item{
		{Name: "ok", File: "ok.sh", Type: "rootscript", ParallelGroup: "a"},
		{Name: "boom", File: "fail.sh", Type: "rootscript", ParallelGroup: "a", FailPolicy: "failure_is_not_an_option"},
	}
	if err := m.ProcessItems(items, "userland"); err == nil {
		t.Fatalf("strict policy should have aborted")
	}
}

func TestManager_ParallelGroupRespectsFailPolicyFailable(t *testing.T) {
	cfg := config.NewConfig()
	cfg.DownloadMaxConcurrency = 4
	logger := utils.NewLogger(false, false)

	inst := &fakeInstaller{}
	m := NewManager(&fakeDownloader{}, inst, cfg, logger)

	items := []config.Item{
		{Name: "ok", File: "ok.sh", Type: "rootscript", ParallelGroup: "a", FailPolicy: "failable"},
		{Name: "boom", File: "fail.sh", Type: "rootscript", ParallelGroup: "a", FailPolicy: "failable"},
	}
	if err := m.ProcessItems(items, "userland"); err != nil {
		t.Fatalf("failable should swallow group errors: %v", err)
	}
}

func TestManager_NoParallelGroupRunsSerially(t *testing.T) {
	// Regression guard: items without parallel_group must still run one-at-a-time.
	cfg := config.NewConfig()
	logger := utils.NewLogger(false, false)

	inst := &recordingInstaller{delay: 30 * time.Millisecond}
	m := NewManager(&fakeDownloader{}, inst, cfg, logger)

	items := []config.Item{
		{Name: "a", File: "a.sh", Type: "rootscript"},
		{Name: "b", File: "b.sh", Type: "rootscript"},
		{Name: "c", File: "c.sh", Type: "rootscript"},
	}
	if err := m.ProcessItems(items, "userland"); err != nil {
		t.Fatalf("ProcessItems: %v", err)
	}
	if atomic.LoadInt32(&inst.maxInFlight) != 1 {
		t.Fatalf("expected sequential execution (maxInFlight=1), got %d", inst.maxInFlight)
	}
}
